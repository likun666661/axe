package srd

// A self-contained pure-Go 1.20+ utility for applying human-readable
// “pseudo-diff” patch files to a collection of text files.

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type FileSystem interface {
	Open(string) (string, error)
	Write(string, string) error
	Remove(string) error
}

// --------------------------------------------------------------------------- //
//
//	Domain objects
//
// --------------------------------------------------------------------------- //
type ActionType string

const (
	ActionAdd    ActionType = "add"
	ActionDelete ActionType = "delete"
	ActionUpdate ActionType = "update"
	ModeKeep     string     = "keep"
	ModeAdd      string     = "add"
	ModeDelete   string     = "delete"
)

type FileChange struct {
	Type       ActionType
	OldContent *string
	NewContent *string
	MovePath   string
}

type Commit struct {
	Changes map[string]FileChange
}

// --------------------------------------------------------------------------- //
//  Exceptions
// --------------------------------------------------------------------------- //

type DiffError struct {
	msg string
}

func (e *DiffError) Error() string { return e.msg }

func diffErrorf(format string, a ...any) *DiffError {
	return &DiffError{msg: fmt.Sprintf(format, a...)}
}

// --------------------------------------------------------------------------- //
//  Helper dataclasses used while parsing patches
// --------------------------------------------------------------------------- //

type Chunk struct {
	OrigIndex int
	DelLines  []string
	InsLines  []string
}

type PatchAction struct {
	Type     ActionType
	NewFile  *string
	Chunks   []Chunk
	MovePath string
}

type Patch struct {
	Actions map[string]*PatchAction
}

// --------------------------------------------------------------------------- //
//  Patch text parser
// --------------------------------------------------------------------------- //

type Parser struct {
	CurrentFiles map[string]string
	Lines        []string
	Index        int
	Patch        Patch
	Fuzz         int
}

// ------------- low-level helpers -------------------------------------- //
func (p *Parser) curLine() (string, error) {
	if p.Index >= len(p.Lines) {
		return "", diffErrorf("Unexpected end of input while parsing patch")
	}
	return p.Lines[p.Index], nil
}

func norm(line string) string {
	// Strip CR so comparisons work for both LF and CRLF input.
	for strings.HasSuffix(line, "\r") {
		line = strings.TrimSuffix(line, "\r")
	}
	return line
}

// ------------- scanning convenience ----------------------------------- //
func (p *Parser) isDone(prefixes ...string) bool {
	if p.Index >= len(p.Lines) {
		return true
	}
	if len(prefixes) > 0 {
		cl, _ := p.curLine()
		cl = norm(cl)
		for _, pre := range prefixes {
			if strings.HasPrefix(cl, pre) {
				return true
			}
		}
	}
	return false
}

func (p *Parser) startsWith(prefix string) bool {
	cl, _ := p.curLine()
	return strings.HasPrefix(norm(cl), prefix)
}

func (p *Parser) readStr(prefix string) (string, bool, error) {
	if prefix == "" {
		return "", false, errors.New("read_str() requires a non-empty prefix")
	}
	if p.Index >= len(p.Lines) {
		return "", false, diffErrorf("Unexpected end of input while parsing patch")
	}
	cl := p.Lines[p.Index]
	if strings.HasPrefix(norm(cl), prefix) {
		text := cl[len(prefix):] // raw suffix, do NOT normalize
		p.Index++
		return text, true, nil
	}
	return "", false, nil
}

func (p *Parser) readLine() (string, error) {
	line, err := p.curLine()
	if err != nil {
		return "", err
	}
	p.Index++
	return line, nil
}

// ------------- public entry point -------------------------------------- //
func (p *Parser) parse() error {
	for !p.isDone("*** End Patch") {
		// ---------- UPDATE ---------- //
		if path, ok, err := p.readStr("*** Update File: "); err != nil {
			return err
		} else if ok {
			if _, exists := p.Patch.Actions[path]; exists {
				return diffErrorf("Duplicate update for file: %s", path)
			}
			moveTo, _, err := p.readStr("*** Move to: ")
			if err != nil {
				return err
			}
			// Allow update on non-existent files (will succeed if only additions)
			text, fileExists := p.CurrentFiles[path]
			if !fileExists {
				text = ""
			}
			action, err := p.parseUpdateFile(text)
			if err != nil {
				return err
			}
			// If file didn't exist and we have successful parse, convert to Add if appropriate
			if !fileExists && isOnlyAdditions(action) {
				content := buildContentFromAdditions(action)
				action = PatchAction{Type: ActionAdd, NewFile: &content}
			} else if !fileExists {
				return diffErrorf("Update File Error - missing file: %s (and update contains context/deletions)", path)
			}
			action.MovePath = moveTo
			p.Patch.Actions[path] = &action
			continue
		}

		// ---------- DELETE ---------- //
		if path, ok, err := p.readStr("*** Delete File: "); err != nil {
			return err
		} else if ok {
			if _, exists := p.Patch.Actions[path]; exists {
				return diffErrorf("Duplicate delete for file: %s", path)
			}
			if _, ok := p.CurrentFiles[path]; !ok {
				return diffErrorf("Delete File Error - missing file: %s", path)
			}
			p.Patch.Actions[path] = &PatchAction{Type: ActionDelete}
			continue
		}

		// ---------- ADD ---------- //
		if path, ok, err := p.readStr("*** Add File: "); err != nil {
			return err
		} else if ok {
			if _, exists := p.Patch.Actions[path]; exists {
				return diffErrorf("Duplicate add for file: %s", path)
			}
			// Allow Add File to rewrite existing files (Best Practice 3)
			// This is preferred for complete file rewrites
			action, err := p.parseAddFile()
			if err != nil {
				return err
			}
			p.Patch.Actions[path] = &action
			continue
		}

		cl, _ := p.curLine()
		return diffErrorf("Unknown line while parsing: %s", cl)
	}

	if !p.startsWith("*** End Patch") {
		return diffErrorf("Missing *** End Patch sentinel")
	}
	p.Index++ // consume sentinel
	return nil
}

// ------------- section parsers ---------------------------------------- //
func (p *Parser) parseUpdateFile(text string) (PatchAction, error) {
	action := PatchAction{Type: ActionUpdate}
	lines := strings.Split(text, "\n")
	index := 0
	for !p.isDone("*** End Patch", "*** Update File:", "*** Delete File:", "*** Add File:", "*** End of File") {
		nextCtx, chunks, endIdx, eof, err := peekNextSection(p.Lines, p.Index)
		if err != nil {
			return action, err
		}
		newIndex, fuzz := findContext(lines, nextCtx, index, eof)
		if newIndex == -1 {
			ctxTxt := strings.Join(nextCtx, "\n")
			prefix := ""
			if eof {
				prefix = "EOF "
			}
			return action, diffErrorf("Invalid %scontext at %d:\n%s", prefix, index, ctxTxt)
		}
		p.Fuzz += fuzz
		for _, ch := range chunks {
			ch.OrigIndex += newIndex
			action.Chunks = append(action.Chunks, ch)
		}
		index = newIndex + len(nextCtx)
		p.Index = endIdx
	}
	return action, nil
}

func (p *Parser) parseAddFile() (PatchAction, error) {
	var lines []string
	for !p.isDone("*** End Patch", "*** Update File:", "*** Delete File:", "*** Add File:") {
		s, err := p.readLine()
		if err != nil {
			return PatchAction{}, err
		}

		// Silently skip @@ lines for error tolerance
		normalized := norm(s)
		if strings.HasPrefix(normalized, "@@") || normalized == "@@" {
			continue
		}

		// For error tolerance, if line doesn't start with +, prepend it
		if !strings.HasPrefix(s, "+") {
			s = "+" + s
		}
		lines = append(lines, s[1:]) // strip leading '+'
	}
	content := strings.Join(lines, "\n")
	return PatchAction{Type: ActionAdd, NewFile: &content}, nil
}

// --------------------------------------------------------------------------- //
//  Helper functions
// --------------------------------------------------------------------------- //

// isOnlyAdditions checks if an update action contains only additions (no deletions or context needed)
func isOnlyAdditions(action PatchAction) bool {
	if action.Type != ActionUpdate {
		return false
	}
	for _, chunk := range action.Chunks {
		if len(chunk.DelLines) > 0 {
			return false
		}
		// If OrigIndex > 0, it means there's context before it
		if chunk.OrigIndex > 0 {
			return false
		}
	}
	return true
}

// buildContentFromAdditions extracts all additions from an update action to create file content
func buildContentFromAdditions(action PatchAction) string {
	var lines []string
	for _, chunk := range action.Chunks {
		lines = append(lines, chunk.InsLines...)
	}
	return strings.Join(lines, "\n")
}

func findContextCore(lines, context []string, start int) (int, int) {
	if len(context) == 0 {
		return start, 0
	}
	// Strip match (primary - always ignore whitespace for context lines)
	for i := start; i+len(context) <= len(lines); i++ {
		if slicesEqualStrip(lines[i:i+len(context)], context) {
			return i, 0
		}
	}
	// rstrip match (fallback)
	for i := start; i+len(context) <= len(lines); i++ {
		if slicesEqualRStrip(lines[i:i+len(context)], context) {
			return i, 1
		}
	}
	// Strict match (last resort)
	for i := start; i+len(context) <= len(lines); i++ {
		if slicesEqual(lines[i:i+len(context)], context) {
			return i, 100
		}
	}
	return -1, 0
}

func findContext(lines, context []string, start int, eof bool) (int, int) {
	if eof {
		pos := len(lines) - len(context)
		if pos < 0 {
			pos = 0
		}
		if newIndex, fuzz := findContextCore(lines, context, pos); newIndex != -1 {
			return newIndex, fuzz
		}
		if newIndex, fuzz := findContextCore(lines, context, start); newIndex != -1 {
			return newIndex, fuzz + 10_000
		}
		return -1, 0
	}
	return findContextCore(lines, context, start)
}

// Replace the entire peekNextSection with this version.
func peekNextSection(lines []string, index int) ([]string, []Chunk, int, bool, error) {
	var old []string
	var delLines []string
	var insLines []string
	var chunks []Chunk
	mode := ModeKeep
	origIndex := index

	startsWithAny := func(s string, prefixes ...string) bool {
		for _, pre := range prefixes {
			if strings.HasPrefix(s, pre) {
				return true
			}
		}
		return false
	}

	for index < len(lines) {
		raw := lines[index]
		s := norm(raw) // normalize for sentinel checks

		if startsWithAny(s,
			"*** End Patch",
			"*** Update File:",
			"*** Delete File:",
			"*** Add File:",
			"*** End of File",
		) {
			break
		}
		if s == "***" {
			break
		}

		// Silently skip @@ lines for error tolerance
		if strings.HasPrefix(s, "@@") || s == "@@" {
			index++
			continue
		}

		// Silently skip lines starting with *** that we don't recognize (error tolerance)
		if strings.HasPrefix(s, "***") {
			index++
			continue
		}

		index++

		lastMode := mode
		// Use the *raw* line for hunk markers so leading +/-/space is read correctly.
		line := raw
		if line == "" {
			line = " "
		}

		// Skip empty or whitespace-only lines for error tolerance
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case '+':
			mode = ModeAdd
		case '-':
			mode = ModeDelete
		case ' ':
			mode = ModeKeep
		default:
			// For error tolerance, treat unrecognized lines as context lines
			mode = ModeKeep
			line = " " + line
		}
		line = line[1:]

		if mode == ModeKeep && lastMode != mode {
			if len(insLines) > 0 || len(delLines) > 0 {
				chunks = append(chunks, Chunk{
					OrigIndex: len(old) - len(delLines),
					DelLines:  append([]string{}, delLines...),
					InsLines:  append([]string{}, insLines...),
				})
			}
			delLines = nil
			insLines = nil
		}

		switch mode {
		case ModeDelete:
			delLines = append(delLines, line)
			old = append(old, line)
		case ModeAdd:
			insLines = append(insLines, line)
		case ModeKeep:
			old = append(old, line)
		}
	}

	if len(insLines) > 0 || len(delLines) > 0 {
		chunks = append(chunks, Chunk{
			OrigIndex: len(old) - len(delLines),
			DelLines:  append([]string{}, delLines...),
			InsLines:  append([]string{}, insLines...),
		})
	}

	// CR-safe check for EOF sentinel
	if index < len(lines) && norm(lines[index]) == "*** End of File" {
		index++
		return old, chunks, index, true, nil
	}

	if index == origIndex {
		return nil, nil, 0, false, diffErrorf("Nothing in this section")
	}
	return old, chunks, index, false, nil
}

// --------------------------------------------------------------------------- //
//  Patch → Commit and Commit application
// --------------------------------------------------------------------------- //

func getUpdatedFile(text string, action PatchAction, path string) (string, error) {
	if action.Type != ActionUpdate {
		return "", diffErrorf("_get_updated_file called with non-update action")
	}
	origLines := strings.Split(text, "\n")
	var destLines []string
	origIndex := 0

	for _, chunk := range action.Chunks {
		if chunk.OrigIndex > len(origLines) {
			return "", diffErrorf("%s: chunk.orig_index %d exceeds file length", path, chunk.OrigIndex)
		}
		if origIndex > chunk.OrigIndex {
			return "", diffErrorf("%s: overlapping chunks at %d > %d", path, origIndex, chunk.OrigIndex)
		}
		destLines = append(destLines, origLines[origIndex:chunk.OrigIndex]...)
		origIndex = chunk.OrigIndex
		destLines = append(destLines, chunk.InsLines...)
		origIndex += len(chunk.DelLines)
	}
	destLines = append(destLines, origLines[origIndex:]...)
	return strings.Join(destLines, "\n"), nil
}

func patchToCommit(patch Patch, orig map[string]string) (Commit, error) {
	commit := Commit{Changes: map[string]FileChange{}}
	for path, action := range patch.Actions {
		switch action.Type {
		case ActionDelete:
			old := orig[path]
			commit.Changes[path] = FileChange{
				Type:       ActionDelete,
				OldContent: &old,
			}
		case ActionAdd:
			if action.NewFile == nil {
				return Commit{}, diffErrorf("ADD action without file content")
			}
			// Support Add action to rewrite existing files (Best Practice 3)
			var oldContent *string
			if existing, exists := orig[path]; exists {
				oldContent = &existing
			}
			commit.Changes[path] = FileChange{
				Type:       ActionAdd,
				OldContent: oldContent,
				NewContent: action.NewFile,
			}
		case ActionUpdate:
			newContent, err := getUpdatedFile(orig[path], *action, path)
			if err != nil {
				return Commit{}, err
			}
			old := orig[path]
			nc := newContent
			commit.Changes[path] = FileChange{
				Type:       ActionUpdate,
				OldContent: &old,
				NewContent: &nc,
				MovePath:   action.MovePath,
			}
		}
	}
	return commit, nil
}

// --------------------------------------------------------------------------- //
//  User-facing helpers
// --------------------------------------------------------------------------- //

func textToPatch(text string, orig map[string]string) (Patch, int, error) {
	lines := splitLinesLikePython(text) // preserves blank lines, no strip()

	// Auto-append Begin Patch if missing
	if len(lines) == 0 || !strings.HasPrefix(norm(lines[0]), "*** Begin Patch") {
		lines = append([]string{"*** Begin Patch"}, lines...)
	}

	// Auto-append End Patch if missing
	if len(lines) == 0 || norm(lines[len(lines)-1]) != "*** End Patch" {
		lines = append(lines, "*** End Patch")
	}

	parser := &Parser{
		CurrentFiles: orig,
		Lines:        lines,
		Index:        1,
		Patch:        Patch{Actions: map[string]*PatchAction{}},
		Fuzz:         0,
	}
	if err := parser.parse(); err != nil {
		return Patch{}, 0, err
	}
	return parser.Patch, parser.Fuzz, nil
}

func identifyFilesNeeded(text string) []string {
	lines := splitLinesLikePython(text)
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "*** Update File: ") {
			out = append(out, line[len("*** Update File: "):])
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "*** Delete File: ") {
			out = append(out, line[len("*** Delete File: "):])
		}
	}
	return out
}

func identifyFilesAdded(text string) []string {
	lines := splitLinesLikePython(text)
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "*** Add File: ") {
			out = append(out, line[len("*** Add File: "):])
		}
	}
	return out
}

// --------------------------------------------------------------------------- //
//  File-system helpers
// --------------------------------------------------------------------------- //

type OpenFn func(string) (string, error)
type WriteFn func(string, string) error
type RemoveFn func(string) error

func loadFiles(paths []string, openFn OpenFn) (map[string]string, error) {
	m := make(map[string]string, len(paths))
	for _, p := range paths {
		txt, err := openFn(p)
		if err != nil {
			// File doesn't exist - that's ok, Update File might only have additions
			// We'll validate later if the operation is valid for missing files
			continue
		}
		m[p] = txt
	}
	return m, nil
}

func applyCommit(commit Commit, writeFn WriteFn, removeFn RemoveFn) error {
	for path, change := range commit.Changes {
		switch change.Type {
		case ActionDelete:
			if err := removeFn(path); err != nil {
				return err
			}
		case ActionAdd:
			if change.NewContent == nil {
				return diffErrorf("ADD change for %s has no content", path)
			}
			if err := writeFn(path, *change.NewContent); err != nil {
				return err
			}
		case ActionUpdate:
			if change.NewContent == nil {
				return diffErrorf("UPDATE change for %s has no new content", path)
			}
			target := path
			if change.MovePath != "" {
				target = change.MovePath
			}
			if err := writeFn(target, *change.NewContent); err != nil {
				return err
			}
			if change.MovePath != "" {
				if err := removeFn(path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func processPatch(
	text string,
	openFn OpenFn,
	writeFn WriteFn,
	removeFn RemoveFn,
) (string, error) {
	paths := identifyFilesNeeded(text)
	// Also load files being Added (Best Practice 3: Add can rewrite existing files)
	addPaths := identifyFilesAdded(text)
	allPaths := append(paths, addPaths...)
	orig, err := loadFiles(allPaths, openFn)
	if err != nil {
		return "", err
	}
	patch, _fuzz, err := textToPatch(text, orig)
	if err != nil {
		return "", fmt.Errorf("failed to parse patch: %w", err)
	}
	commit, err := patchToCommit(patch, orig)
	if err != nil {
		return "", fmt.Errorf("failed to convert patch to commit: %w", err)
	}
	if err := applyCommit(commit, writeFn, removeFn); err != nil {
		return "", fmt.Errorf("failed to apply commit: %w", err)
	}
	_ = _fuzz // kept for parity; could be logged if desired
	return "Done!", nil
}

func ApplyPatch(cc FileSystem, patchText string) (string, error) {
	// remove newlines at the beginning and end
	patchText = strings.TrimSpace(patchText)
	patchText += "\n"
	return processPatch(patchText, cc.Open, cc.Write, cc.Remove)
}

// --------------------------------------------------------------------------- //
//  Utilities (parity helpers)
// --------------------------------------------------------------------------- //

// It splits on \n, \r\n, and \r; does not keep the separators;
// and behaves like Python's str.splitlines(keepends=False).
func splitLinesLikePython(s string) []string {
	var lines []string
	start := 0
	i := 0
	for i < len(s) {
		switch s[i] {
		case '\n':
			lines = append(lines, s[start:i])
			i++
			start = i
		case '\r':
			lines = append(lines, s[start:i])
			// Treat \r\n as a single line break
			if i+1 < len(s) && s[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
			start = i
		default:
			i++
		}
	}
	// Append trailing fragment if the string didn't end with a linebreak
	if start != len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func rstripSpaces(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func slicesEqualRStrip(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if rstripSpaces(a[i]) != rstripSpaces(b[i]) {
			return false
		}
	}
	return true
}

func slicesEqualStrip(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
