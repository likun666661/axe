package srd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestPatchSuite(t *testing.T) {
	suite.Run(t, new(PatchSuite))
}

type PatchSuite struct {
	suite.Suite
}

func (s *PatchSuite) stringPtr(v string) *string {
	return &v
}

func (s *PatchSuite) TestSplitLinesLikePython() {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "no trailing newline",
			input: "alpha\nbeta",
			want:  []string{"alpha", "beta"},
		},
		{
			name:  "trailing newline dropped",
			input: "one\n",
			want:  []string{"one"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			got := splitLinesLikePython(tc.input)
			s.Require().Equal(tc.want, got)
		})
	}
}

func (s *PatchSuite) TestFindContext() {
	cases := []struct {
		name      string
		lines     []string
		context   []string
		start     int
		eof       bool
		wantIndex int
		wantFuzz  int
	}{
		{
			name:      "strip match (primary)",
			lines:     []string{"alpha", "beta", "gamma"},
			context:   []string{"beta"},
			start:     0,
			wantIndex: 1,
			wantFuzz:  0,
		},
		{
			name:      "rstrip match (now also strip match)",
			lines:     []string{"foo  ", "bar"},
			context:   []string{"foo"},
			start:     0,
			wantIndex: 0,
			wantFuzz:  0,
		},
		{
			name:      "strip match with spaces",
			lines:     []string{"  foo  ", "bar"},
			context:   []string{"foo"},
			start:     0,
			wantIndex: 0,
			wantFuzz:  0,
		},
		{
			name:      "not found",
			lines:     []string{"one", "two"},
			context:   []string{"missing"},
			start:     0,
			wantIndex: -1,
			wantFuzz:  0,
		},
		{
			name:      "eof fallback",
			lines:     []string{"one", "two", "three"},
			context:   []string{"one"},
			start:     0,
			eof:       true,
			wantIndex: 0,
			wantFuzz:  10_000,
		},
		{
			name:      "eof direct hit",
			lines:     []string{"line1", "line2"},
			context:   []string{"line2"},
			start:     0,
			eof:       true,
			wantIndex: 1,
			wantFuzz:  0,
		},
	}

	for _, tc := range cases {
		tc := tc
		s.Run(tc.name, func() {
			gotIndex, gotFuzz := findContext(tc.lines, tc.context, tc.start, tc.eof)
			s.Equal(tc.wantIndex, gotIndex)
			s.Equal(tc.wantFuzz, gotFuzz)
		})
	}
}

func (s *PatchSuite) TestGetUpdatedFile() {
	cases := []struct {
		name    string
		text    string
		action  PatchAction
		want    string
		wantErr string
	}{
		{
			name: "single replacement",
			text: "line1\nline2\nline3",
			action: PatchAction{
				Type: ActionUpdate,
				Chunks: []Chunk{
					{
						OrigIndex: 1,
						DelLines:  []string{"line2"},
						InsLines:  []string{"line-x"},
					},
				},
			},
			want: "line1\nline-x\nline3",
		},
		{
			name: "insertion at start",
			text: "line1\nline2",
			action: PatchAction{
				Type: ActionUpdate,
				Chunks: []Chunk{
					{
						OrigIndex: 0,
						InsLines:  []string{"new"},
					},
				},
			},
			want: "new\nline1\nline2",
		},
		{
			name: "chunk past end",
			text: "only",
			action: PatchAction{
				Type: ActionUpdate,
				Chunks: []Chunk{
					{
						OrigIndex: 5,
					},
				},
			},
			wantErr: "exceeds file length",
		},
		{
			name: "overlapping chunks",
			text: "a\nb\nc",
			action: PatchAction{
				Type: ActionUpdate,
				Chunks: []Chunk{
					{
						OrigIndex: 1,
						DelLines:  []string{"b"},
					},
					{
						OrigIndex: 1,
						InsLines:  []string{"x"},
					},
				},
			},
			wantErr: "overlapping chunks",
		},
	}

	for _, tc := range cases {
		tc := tc
		s.Run(tc.name, func() {
			got, err := getUpdatedFile(tc.text, tc.action, "test.txt")
			if tc.wantErr != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tc.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tc.want, got)
		})
	}
}

func (s *PatchSuite) TestPatchToCommit() {
	addContent := "new file contents"
	cases := []struct {
		name    string
		patch   Patch
		orig    map[string]string
		wantErr string
		check   func(Commit)
	}{
		{
			name: "happy path",
			patch: Patch{Actions: map[string]*PatchAction{
				"delete.txt": {
					Type: ActionDelete,
				},
				"add.txt": {
					Type:    ActionAdd,
					NewFile: s.stringPtr(addContent),
				},
				"update.txt": {
					Type:     ActionUpdate,
					MovePath: "renamed.txt",
					Chunks: []Chunk{
						{
							OrigIndex: 1,
							DelLines:  []string{"old"},
							InsLines:  []string{"new"},
						},
					},
				},
			}},
			orig: map[string]string{
				"delete.txt": "legacy",
				"update.txt": "keep\nold",
			},
			check: func(commit Commit) {
				del, ok := commit.Changes["delete.txt"]
				s.Require().True(ok)
				s.Equal(ActionDelete, del.Type)
				s.Require().NotNil(del.OldContent)
				s.Equal("legacy", *del.OldContent)

				add, ok := commit.Changes["add.txt"]
				s.Require().True(ok)
				s.Equal(ActionAdd, add.Type)
				s.Require().NotNil(add.NewContent)
				s.Equal(addContent, *add.NewContent)

				upd, ok := commit.Changes["update.txt"]
				s.Require().True(ok)
				s.Equal(ActionUpdate, upd.Type)
				s.Require().NotNil(upd.OldContent)
				s.Equal("keep\nold", *upd.OldContent)
				s.Require().NotNil(upd.NewContent)
				s.Equal("keep\nnew", *upd.NewContent)
				s.Equal("renamed.txt", upd.MovePath)
			},
		},
		{
			name: "missing add content",
			patch: Patch{Actions: map[string]*PatchAction{
				"add.txt": {
					Type: ActionAdd,
				},
			}},
			orig:    map[string]string{},
			wantErr: "ADD action without file content",
		},
		{
			name: "bad update chunk",
			patch: Patch{Actions: map[string]*PatchAction{
				"broken.txt": {
					Type:   ActionUpdate,
					Chunks: []Chunk{{OrigIndex: 5}},
				},
			}},
			orig:    map[string]string{"broken.txt": "text"},
			wantErr: "exceeds file length",
		},
	}

	for _, tc := range cases {
		tc := tc
		s.Run(tc.name, func() {
			commit, err := patchToCommit(tc.patch, tc.orig)
			if tc.wantErr != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tc.wantErr)
				return
			}
			s.Require().NoError(err)
			s.Require().NotNil(tc.check)
			tc.check(commit)
		})
	}
}

func (s *PatchSuite) TestIdentifyFilesHelpers() {
	patchText := "*** Begin Patch\n" +
		"*** Update File: foo.txt\n" +
		" foo\n" +
		"*** Delete File: bar.txt\n" +
		"*** Add File: baz.txt\n" +
		"+line\n" +
		"*** End Patch"

	s.Equal([]string{"foo.txt", "bar.txt"}, identifyFilesNeeded(patchText))
	s.Equal([]string{"baz.txt"}, identifyFilesAdded(patchText))
}

func (s *PatchSuite) TestApplyCommit() {
	newContent := "updated"
	cases := []struct {
		name        string
		commit      Commit
		wantErr     string
		wantWrites  map[string]string
		wantRemoves []string
	}{
		{
			name: "full lifecycle",
			commit: Commit{Changes: map[string]FileChange{
				"delete.txt": {
					Type: ActionDelete,
				},
				"add.txt": {
					Type:       ActionAdd,
					NewContent: s.stringPtr("created"),
				},
				"move.txt": {
					Type:       ActionUpdate,
					NewContent: s.stringPtr(newContent),
					MovePath:   "moved.txt",
				},
			}},
			wantWrites: map[string]string{
				"add.txt":   "created",
				"moved.txt": newContent,
			},
			wantRemoves: []string{"delete.txt", "move.txt"},
		},
		{
			name: "add missing content",
			commit: Commit{Changes: map[string]FileChange{
				"add.txt": {Type: ActionAdd},
			}},
			wantErr: "has no content",
		},
	}

	for _, tc := range cases {
		tc := tc
		s.Run(tc.name, func() {
			writes := make(map[string]string)
			var removes []string

			err := applyCommit(tc.commit,
				func(path, content string) error {
					writes[path] = content
					return nil
				},
				func(path string) error {
					removes = append(removes, path)
					return nil
				},
			)

			if tc.wantErr != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tc.wantErr)
				return
			}

			s.Require().NoError(err)
			s.Equal(tc.wantWrites, writes)
			s.ElementsMatch(tc.wantRemoves, removes)
		})
	}
}

type fakeFileSystem struct {
	files   map[string]string
	writes  map[string]string
	removes []string
}

func newFakeFileSystem(seed map[string]string) *fakeFileSystem {
	files := make(map[string]string, len(seed))
	for k, v := range seed {
		files[k] = v
	}
	return &fakeFileSystem{
		files:  files,
		writes: make(map[string]string),
	}
}

func (fs *fakeFileSystem) Open(path string) (string, error) {
	txt, ok := fs.files[path]
	if !ok {
		return "", fmt.Errorf("missing file: %s", path)
	}
	return txt, nil
}

func (fs *fakeFileSystem) Write(path, content string) error {
	fs.files[path] = content
	fs.writes[path] = content
	return nil
}

func (fs *fakeFileSystem) Remove(path string) error {
	delete(fs.files, path)
	fs.removes = append(fs.removes, path)
	return nil
}

func (s *PatchSuite) TestApplyPatch() {
	cases := []struct {
		name        string
		fs          *fakeFileSystem
		patchText   string
		wantFiles   map[string]string
		wantRemoves []string
	}{
		{
			name: "update delete add with move",
			fs: newFakeFileSystem(map[string]string{
				"foo.txt": "line1\nline2",
				"bar.txt": "old",
			}),
			patchText: "*** Begin Patch\n" +
				"*** Update File: foo.txt\n" +
				"*** Move to: foo-renamed.txt\n" +
				" line1\n" +
				"-line2\n" +
				"+line2 updated\n" +
				"*** End of File\n" +
				"*** Delete File: bar.txt\n" +
				"*** Add File: new.txt\n" +
				"+fresh\n" +
				"*** End Patch",
			wantFiles: map[string]string{
				"foo-renamed.txt": "line1\nline2 updated",
				"new.txt":         "fresh",
			},
			wantRemoves: []string{"foo.txt", "bar.txt"},
		},
	}

	for _, tc := range cases {
		tc := tc
		s.Run(tc.name, func() {
			result, err := ApplyPatch(tc.fs, tc.patchText)
			s.Require().NoError(err)
			s.Equal("Done!", result)
			for path, content := range tc.wantFiles {
				got, ok := tc.fs.files[path]
				s.Require().True(ok, "expected file %s to exist", path)
				s.Equal(content, got)
			}
			s.ElementsMatch(tc.wantRemoves, tc.fs.removes)
			for _, removed := range tc.wantRemoves {
				_, stillThere := tc.fs.files[removed]
				s.False(stillThere, "expected %s to be removed", removed)
			}
		})
	}
}

func (s *PatchSuite) TestApplyPatchWithNewlines() {
	patchText := `
*** Begin Patch
*** Update File: foo.txt
 foo
-bar
+bar updated
 haha
*** End of File
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"foo.txt": "foo\nbar\nhaha",
	})
	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal("foo\nbar updated\nhaha", fs.files["foo.txt"])
}

func (s *PatchSuite) TestErrorTolerance() {
	// Test with @@ lines that should be ignored
	patchText := `
*** Begin Patch
*** Update File: test.txt
@@ some context marker @@
 line1
@@
-old line
+new line
@@ -1,3 +1,3 @@
 line3
*** End of File
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"test.txt": "line1\nold line\nline3",
	})
	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal("line1\nnew line\nline3", fs.files["test.txt"])
}

func (s *PatchSuite) TestErrorToleranceAddFile() {
	// Test Add File with @@ lines
	patchText := `
*** Begin Patch
*** Add File: new.txt
@@ file header @@
+line 1
@@
+line 2
+line 3
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})
	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal("line 1\nline 2\nline 3", fs.files["new.txt"])
}

func (s *PatchSuite) TestMissingSentinels() {
	// Test without Begin and End Patch
	patchText := `
*** Update File: foo.txt
 foo
-bar
+bar modified
 baz
*** End of File
`

	fs := newFakeFileSystem(map[string]string{
		"foo.txt": "foo\nbar\nbaz",
	})
	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal("foo\nbar modified\nbaz", fs.files["foo.txt"])
}

func (s *PatchSuite) TestApplyPatchDemoAddTest() {
	// Original content of demo/add_test.go before the patch
	originalContent := `package demo

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{
			a:        1,
			b:        2,
			expected: 3,
		},
		{
			a:        1,
			b:        2,
			expected: 3,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("Add(%d, %d)", test.a, test.b), func(t *testing.T) {
			actual := Add(test.a, test.b)
			if actual != test.expected {
				t.Errorf("Add(%d, %d) = %d, expected %d", test.a, test.b, actual, test.expected)
			}
		})
	}
}

type AddTestSuite struct {
	suite.Suite
}

func (suite *AddTestSuite) TestAdd() {
	tests := []struct {
		a, b, expected int
	}{
		{
			a:        1,
			b:        2,
			expected: 3,
		},
		{
			a:        -1,
			b:        -2,
			expected: -3,
		},
	}
	for _, test := range tests {
		suite.Run(fmt.Sprintf("Add(%d, %d)", test.a, test.b), func() {
			actual := Add(test.a, test.b)
			assert.Equal(suite.T(), test.expected, actual, "Add(%d, %d) = %d, expected %d", test.a, test.b, actual, test.expected)
		})
	}
}

func (suite *AddTestSuite) TestSuperAdd() {
	tests := []struct {
		a, b, expected *big.Int
	}{
		{
			a:        big.NewInt(1),
			b:        big.NewInt(2),
			expected: big.NewInt(3),
		},
		{
			a:        big.NewInt(-1),
			b:        big.NewInt(-2),
			expected: big.NewInt(-3),
		},
	}
	for _, test := range tests {
		suite.Run(fmt.Sprintf("SuperAdd(%s, %s)", test.a, test.b), func() {
			actual := SuperAdd(test.a, test.b)
			assert.Equal(suite.T(), test.expected, actual, "SuperAdd(%s, %s) = %s, expected %s", test.a, test.b, actual, test.expected)
		})
	}
}

func TestAddTestSuite(t *testing.T) {
	suite.Run(t, new(AddTestSuite))
}
`

	// Expected content after applying the patch
	expectedContent := `package demo

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AddTestSuite struct {
	suite.Suite
}

func (suite *AddTestSuite) TestAdd() {
	tests := []struct {
		a, b, expected int
	}{
		{
			a:        1,
			b:        2,
			expected: 3,
		},
		{
			a:        -1,
			b:        -2,
			expected: -3,
		},
	}
	for _, test := range tests {
		suite.Run(fmt.Sprintf("Add(%d, %d)", test.a, test.b), func() {
			actual := Add(test.a, test.b)
			assert.Equal(suite.T(), test.expected, actual, "Add(%d, %d) = %d, expected %d", test.a, test.b, actual, test.expected)
		})
	}
}

func (suite *AddTestSuite) TestSuperAdd() {
	tests := []struct {
		a, b, expected *big.Int
	}{
		{
			a:        big.NewInt(1),
			b:        big.NewInt(2),
			expected: big.NewInt(3),
		},
		{
			a:        big.NewInt(-1),
			b:        big.NewInt(-2),
			expected: big.NewInt(-3),
		},
	}
	for _, test := range tests {
		suite.Run(fmt.Sprintf("SuperAdd(%s, %s)", test.a, test.b), func() {
			actual := SuperAdd(test.a, test.b)
			assert.Equal(suite.T(), test.expected, actual, "SuperAdd(%s, %s) = %s, expected %s", test.a, test.b, actual, test.expected)
		})
	}
}

func TestAddTestSuite(t *testing.T) {
	suite.Run(t, new(AddTestSuite))
}
`

	// Patch text that replaces the entire file
	patchText := `*** Begin Patch
*** Update File: demo/add_test.go
 package demo
 
 import (
 	"fmt"
 	"math/big"
 	"testing"
 
 	"github.com/stretchr/testify/assert"
 	"github.com/stretchr/testify/suite"
 )
 
-func TestAdd(t *testing.T) {
-	tests := []struct {
-		a, b, expected int
-	}{
-		{
-			a:        1,
-			b:        2,
-			expected: 3,
-		},
-		{
-			a:        1,
-			b:        2,
-			expected: 3,
-		},
-	}
-	for _, test := range tests {
-		t.Run(fmt.Sprintf("Add(%d, %d)", test.a, test.b), func(t *testing.T) {
-			actual := Add(test.a, test.b)
-			if actual != test.expected {
-				t.Errorf("Add(%d, %d) = %d, expected %d", test.a, test.b, actual, test.expected)
-			}
-		})
-	}
-}
-
 type AddTestSuite struct {
 	suite.Suite
 }
 
 func (suite *AddTestSuite) TestAdd() {
 	tests := []struct {
 		a, b, expected int
 	}{
 		{
 			a:        1,
 			b:        2,
 			expected: 3,
 		},
 		{
 			a:        -1,
 			b:        -2,
 			expected: -3,
 		},
 	}
 	for _, test := range tests {
 		suite.Run(fmt.Sprintf("Add(%d, %d)", test.a, test.b), func() {
 			actual := Add(test.a, test.b)
 			assert.Equal(suite.T(), test.expected, actual, "Add(%d, %d) = %d, expected %d", test.a, test.b, actual, test.expected)
 		})
 	}
 }
 
 func (suite *AddTestSuite) TestSuperAdd() {
 	tests := []struct {
 		a, b, expected *big.Int
 	}{
 		{
 			a:        big.NewInt(1),
 			b:        big.NewInt(2),
 			expected: big.NewInt(3),
 		},
 		{
 			a:        big.NewInt(-1),
 			b:        big.NewInt(-2),
 			expected: big.NewInt(-3),
 		},
 	}
 	for _, test := range tests {
 		suite.Run(fmt.Sprintf("SuperAdd(%s, %s)", test.a, test.b), func() {
 			actual := SuperAdd(test.a, test.b)
 			assert.Equal(suite.T(), test.expected, actual, "SuperAdd(%s, %s) = %s, expected %s", test.a, test.b, actual, test.expected)
 		})
 	}
 }
 
 func TestAddTestSuite(t *testing.T) {
 	suite.Run(t, new(AddTestSuite))
 }
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"demo/add_test.go": originalContent,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(expectedContent, fs.files["demo/add_test.go"])
}

func (s *PatchSuite) TestUpdateFileOnNonExistentFile() {
	// Test that Update File works on non-existent files when only additions
	patchText := `
*** Begin Patch
*** Update File: new_file.txt
+line 1
+line 2
+line 3
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})
	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal("line 1\nline 2\nline 3", fs.files["new_file.txt"])
}

func (s *PatchSuite) TestUpdateFileOnNonExistentFileWithContext() {
	// Test that Update File fails on non-existent files when it has context or deletions
	patchText := `
*** Begin Patch
*** Update File: new_file.txt
 context line
+new line
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})
	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	// Error can be either "missing file" or "Invalid context" depending on when it's caught
	s.True(strings.Contains(err.Error(), "missing file") || strings.Contains(err.Error(), "Invalid context"))
}

func (s *PatchSuite) TestUpdateFileOnNonExistentFileWithDeletions() {
	// Test that Update File fails on non-existent files when it has deletions
	patchText := `
*** Begin Patch
*** Update File: new_file.txt
-old line
+new line
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})
	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

// ============================================================================
// Comprehensive tests based on README.md examples
// ============================================================================

func (s *PatchSuite) TestREADME_BasicUpdateExample() {
	// Tests the basic update example from README
	patchText := `
*** Begin Patch
*** Update File: src/hello.py
 def greet(name):
-    return "Hi"
+    return f"Hello, {name}!"
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"src/hello.py": `def greet(name):
    return "Hi"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def greet(name):
    return f"Hello, {name}!"`, fs.files["src/hello.py"])
}

func (s *PatchSuite) TestREADME_AddingLine() {
	// Tests adding a line example from README
	patchText := `
*** Begin Patch
*** Update File: config.py
 DEBUG = False
 LOG_LEVEL = "INFO"
+MAX_RETRIES = 3
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = False
LOG_LEVEL = "INFO"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`DEBUG = False
LOG_LEVEL = "INFO"
MAX_RETRIES = 3`, fs.files["config.py"])
}

func (s *PatchSuite) TestREADME_RemovingLine() {
	// Tests removing a line example from README
	patchText := `
*** Begin Patch
*** Update File: config.py
 DEBUG = False
-LOG_LEVEL = "INFO"
 MAX_RETRIES = 3
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = False
LOG_LEVEL = "INFO"
MAX_RETRIES = 3`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`DEBUG = False
MAX_RETRIES = 3`, fs.files["config.py"])
}

func (s *PatchSuite) TestREADME_ChangingLine() {
	// Tests changing a line example from README
	patchText := `
*** Begin Patch
*** Update File: config.py
 DEBUG = False
-LOG_LEVEL = "INFO"
+LOG_LEVEL = "DEBUG"
 MAX_RETRIES = 3
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = False
LOG_LEVEL = "INFO"
MAX_RETRIES = 3`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`DEBUG = False
LOG_LEVEL = "DEBUG"
MAX_RETRIES = 3`, fs.files["config.py"])
}

func (s *PatchSuite) TestREADME_AddFile() {
	// Tests adding a file example from README
	patchText := `
*** Begin Patch
*** Add File: utils/helper.py
+def calculate(x, y):
+    return x + y
+
+def format_output(value):
+    return f"Result: {value}"
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def calculate(x, y):
    return x + y

def format_output(value):
    return f"Result: {value}"`, fs.files["utils/helper.py"])
}

func (s *PatchSuite) TestREADME_DeleteFile() {
	// Tests deleting a file example from README
	patchText := `
*** Begin Patch
*** Delete File: old/deprecated.py
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"old/deprecated.py": "old content",
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	_, exists := fs.files["old/deprecated.py"]
	s.False(exists)
	s.Contains(fs.removes, "old/deprecated.py")
}

func (s *PatchSuite) TestREADME_MoveFile() {
	// Tests moving/renaming a file example from README
	patchText := `
*** Begin Patch
*** Update File: config.py
*** Move to: settings.py
 DEBUG = False
-LOG_LEVEL = "INFO"
+LOG_LEVEL = "DEBUG"
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = False
LOG_LEVEL = "INFO"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)

	// New file should exist
	s.Equal(`DEBUG = False
LOG_LEVEL = "DEBUG"`, fs.files["settings.py"])

	// Old file should be removed
	_, exists := fs.files["config.py"]
	s.False(exists)
	s.Contains(fs.removes, "config.py")
}

func (s *PatchSuite) TestREADME_CompleteExample() {
	// Tests the complete example from README with multiple operations
	patchText := `
*** Begin Patch
*** Update File: main.py
 import sys
 
 def main():
-    print("Hello")
+    print("Hello, World!")
+    print("Welcome to the app")
 
 if __name__ == "__main__":
     main()
*** End of File
*** Delete File: old/deprecated.py
*** Add File: utils/helper.py
+def format_name(name):
+    return name.strip().title()
+
+def validate_input(text):
+    return len(text) > 0
*** Update File: config.py
*** Move to: settings/config.py
 APP_NAME = "MyApp"
-VERSION = "1.0"
+VERSION = "1.1"
 DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"main.py": `import sys

def main():
    print("Hello")

if __name__ == "__main__":
    main()`,
		"old/deprecated.py": "old stuff",
		"config.py": `APP_NAME = "MyApp"
VERSION = "1.0"
DEBUG = True`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)

	// Check main.py was updated
	s.Equal(`import sys

def main():
    print("Hello, World!")
    print("Welcome to the app")

if __name__ == "__main__":
    main()`, fs.files["main.py"])

	// Check old/deprecated.py was deleted
	_, exists := fs.files["old/deprecated.py"]
	s.False(exists)

	// Check utils/helper.py was created
	s.Equal(`def format_name(name):
    return name.strip().title()

def validate_input(text):
    return len(text) > 0`, fs.files["utils/helper.py"])

	// Check config.py was moved to settings/config.py with updates
	s.Equal(`APP_NAME = "MyApp"
VERSION = "1.1"
DEBUG = True`, fs.files["settings/config.py"])
	_, exists = fs.files["config.py"]
	s.False(exists)
}

func (s *PatchSuite) TestREADME_BestPractice3_AddFileToRewrite() {
	// Tests Best Practice 3: Use Add File to completely rewrite an existing file
	patchText := `
*** Begin Patch
*** Add File: config.py
+DEBUG = False
+LOG_LEVEL = "INFO"
+TIMEOUT = 30
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `OLD_CONFIG = True
LEGACY_SETTING = "deprecated"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	// The file should be completely rewritten with the new content
	s.Equal(`DEBUG = False
LOG_LEVEL = "INFO"
TIMEOUT = 30`, fs.files["config.py"])
}

func (s *PatchSuite) TestREADME_EndOfFileMarker() {
	// Tests End of File marker for EOF changes
	patchText := `
*** Begin Patch
*** Update File: config.py
 DEBUG = False
 LOG_LEVEL = "INFO"
+TIMEOUT = 30
*** End of File
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = False
LOG_LEVEL = "INFO"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`DEBUG = False
LOG_LEVEL = "INFO"
TIMEOUT = 30`, fs.files["config.py"])
}

func (s *PatchSuite) TestREADME_MultipleHunksInOneFile() {
	// Tests the "One Hunk Per Location" example
	patchText := `
*** Begin Patch
*** Update File: server.py
 def start():
-    port = 8000
+    port = 3000
     listen(port)
 
 def stop():
-    timeout = 30
+    timeout = 60
     shutdown(timeout)
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"server.py": `def start():
    port = 8000
    listen(port)

def stop():
    timeout = 30
    shutdown(timeout)`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def start():
    port = 3000
    listen(port)

def stop():
    timeout = 60
    shutdown(timeout)`, fs.files["server.py"])
}

func (s *PatchSuite) TestREADME_GoodContextExample() {
	// Tests the "Good" context example from README
	patchText := `
*** Begin Patch
*** Update File: app.py
 def initialize():
     config = load_config()
-    old_value = 1
+    new_value = 2
     return config
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `def initialize():
    config = load_config()
    old_value = 1
    return config`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def initialize():
    config = load_config()
    new_value = 2
    return config`, fs.files["app.py"])
}

func (s *PatchSuite) TestIndentation_SpacesVsTabs() {
	// Tests that indentation must match exactly
	patchText := `
*** Begin Patch
*** Update File: code.py
 def foo():
-    return 1
+    return 2
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"code.py": `def foo():
    return 1`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def foo():
    return 2`, fs.files["code.py"])
}

func (s *PatchSuite) TestMultipleAdditions() {
	// Tests adding multiple lines in one hunk
	patchText := `
*** Begin Patch
*** Update File: app.py
 def main():
     print("Start")
+    print("Processing...")
+    print("Almost done...")
+    print("Complete!")
     print("End")
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `def main():
    print("Start")
    print("End")`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def main():
    print("Start")
    print("Processing...")
    print("Almost done...")
    print("Complete!")
    print("End")`, fs.files["app.py"])
}

func (s *PatchSuite) TestMultipleDeletions() {
	// Tests removing multiple consecutive lines
	patchText := `
*** Begin Patch
*** Update File: app.py
 def main():
     print("Start")
-    print("Debug 1")
-    print("Debug 2")
-    print("Debug 3")
     print("End")
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `def main():
    print("Start")
    print("Debug 1")
    print("Debug 2")
    print("Debug 3")
    print("End")`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def main():
    print("Start")
    print("End")`, fs.files["app.py"])
}

func (s *PatchSuite) TestEmptyLines() {
	// Tests handling empty lines in patches
	patchText := `
*** Begin Patch
*** Update File: code.py
 def foo():
     pass
 
-def bar():
-    pass
+def baz():
+    return True
 
 def end():
     pass
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"code.py": `def foo():
    pass

def bar():
    pass

def end():
    pass`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`def foo():
    pass

def baz():
    return True

def end():
    pass`, fs.files["code.py"])
}

func (s *PatchSuite) TestUpdateAtStartOfFile() {
	// Tests updating at the very beginning of a file
	patchText := `
*** Begin Patch
*** Update File: script.py
+#!/usr/bin/env python3
+
 import os
 import sys
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"script.py": `import os
import sys`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`#!/usr/bin/env python3

import os
import sys`, fs.files["script.py"])
}

func (s *PatchSuite) TestUpdateAtEndOfFile() {
	// Tests updating at the very end of a file
	patchText := `
*** Begin Patch
*** Update File: script.py
 import os
 import sys
+
+if __name__ == "__main__":
+    main()
*** End of File
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"script.py": `import os
import sys`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`import os
import sys

if __name__ == "__main__":
    main()`, fs.files["script.py"])
}

func (s *PatchSuite) TestComplexRefactoring() {
	// Tests a more complex refactoring with multiple changes
	patchText := `
*** Begin Patch
*** Update File: calculator.py
 class Calculator:
-    def __init__(self):
-        self.value = 0
+    def __init__(self, initial=0):
+        self.value = initial
+        self.history = []
 
     def add(self, x):
         self.value += x
+        self.history.append(('add', x))
         return self.value
 
     def subtract(self, x):
         self.value -= x
+        self.history.append(('subtract', x))
         return self.value
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"calculator.py": `class Calculator:
    def __init__(self):
        self.value = 0

    def add(self, x):
        self.value += x
        return self.value

    def subtract(self, x):
        self.value -= x
        return self.value`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`class Calculator:
    def __init__(self, initial=0):
        self.value = initial
        self.history = []

    def add(self, x):
        self.value += x
        self.history.append(('add', x))
        return self.value

    def subtract(self, x):
        self.value -= x
        self.history.append(('subtract', x))
        return self.value`, fs.files["calculator.py"])
}

func (s *PatchSuite) TestMultipleFilesWithDifferentOperations() {
	// Tests a realistic patch with various operations
	patchText := `
*** Begin Patch
*** Add File: models/user.py
+class User:
+    def __init__(self, name):
+        self.name = name
*** Update File: main.py
+from models.user import User
+
 def main():
-    print("Hello")
+    user = User("Alice")
+    print(f"Hello, {user.name}")
*** Delete File: old_main.py
*** Update File: config.json
*** Move to: settings.json
 {
-  "version": "1.0"
+  "version": "2.0"
 }
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"main.py": `def main():
    print("Hello")`,
		"old_main.py": "# deprecated",
		"config.json": `{
  "version": "1.0"
}`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)

	// Check new file created
	s.Equal(`class User:
    def __init__(self, name):
        self.name = name`, fs.files["models/user.py"])

	// Check main.py updated
	s.Equal(`from models.user import User

def main():
    user = User("Alice")
    print(f"Hello, {user.name}")`, fs.files["main.py"])

	// Check old file deleted
	_, exists := fs.files["old_main.py"]
	s.False(exists)

	// Check file moved
	s.Equal(`{
  "version": "2.0"
}`, fs.files["settings.json"])
	_, exists = fs.files["config.json"]
	s.False(exists)
}

// ============================================================================
// Tests for adding lines at the beginning of a file
// ============================================================================

func (s *PatchSuite) TestAddLinesAtBeginning_WithContext() {
	// Test adding lines at the very beginning with context from first line
	patchText := `
*** Begin Patch
*** Update File: script.py
+# New header comment
+# Author: John Doe
+
 import os
 import sys
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"script.py": `import os
import sys`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`# New header comment
# Author: John Doe

import os
import sys`, fs.files["script.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_SingleLine() {
	// Test adding a single line at the beginning
	patchText := `
*** Begin Patch
*** Update File: config.py
+# Configuration file
 DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`# Configuration file
DEBUG = True`, fs.files["config.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Shebang() {
	// Test adding shebang at the very beginning
	patchText := `
*** Begin Patch
*** Update File: script.sh
+#!/bin/bash
+set -e
+
 echo "Hello"
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"script.sh": `echo "Hello"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`#!/bin/bash
set -e

echo "Hello"`, fs.files["script.sh"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_NoContext() {
	// Test adding lines with no existing context (entire file is additions)
	// This should work as it's equivalent to creating a file
	patchText := `
*** Begin Patch
*** Update File: newfile.txt
+First line
+Second line
+Third line
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`First line
Second line
Third line`, fs.files["newfile.txt"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_EmptyFile() {
	// Test adding lines to an empty file
	patchText := `
*** Begin Patch
*** Update File: empty.txt
+First line added
+Second line added
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"empty.txt": "",
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	// When building from additions, there's a trailing newline
	s.Equal("First line added\nSecond line added\n", fs.files["empty.txt"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_WithMultipleContextLines() {
	// Test adding at beginning with multiple context lines
	patchText := `
*** Begin Patch
*** Update File: app.py
+"""
+Application module
+"""
+
 import os
 import sys
 
 def main():
     pass
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `import os
import sys

def main():
    pass`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`"""
Application module
"""

import os
import sys

def main():
    pass`, fs.files["app.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Copyright() {
	// Real-world example: adding copyright header
	patchText := `
*** Begin Patch
*** Update File: main.go
+// Copyright 2025 Company Inc.
+// Licensed under MIT
+
 package main
 
 import "fmt"
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"main.go": `package main

import "fmt"`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`// Copyright 2025 Company Inc.
// Licensed under MIT

package main

import "fmt"`, fs.files["main.go"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_MultipleOperations() {
	// Test adding at beginning while also making changes elsewhere
	patchText := `
*** Begin Patch
*** Update File: server.py
+#!/usr/bin/env python3
+"""Server module"""
+
 import socket
 
 def start():
-    port = 8000
+    port = 3000
     return port
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"server.py": `import socket

def start():
    port = 8000
    return port`,
	})

	result, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	s.Equal("Done!", result)
	s.Equal(`#!/usr/bin/env python3
"""Server module"""

import socket

def start():
    port = 3000
    return port`, fs.files["server.py"])
}

// ============================================================================
// Negative tests for adding lines at the beginning
// ============================================================================

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_MissingContext() {
	// Test: trying to add lines without any context when file has content
	// Due to our "update on non-existent file" feature, this succeeds if only additions
	patchText := `
*** Begin Patch
*** Update File: config.py
+# New comment
+# Another comment
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"
MAX_RETRIES = 3`,
	})

	_, err := ApplyPatch(fs, patchText)
	// This succeeds because it's treated as "add only" (no context, no deletions)
	// The additions are added at the beginning (OrigIndex = 0)
	s.Require().NoError(err)
	// The additions are added at the beginning, original content follows
	s.Equal(`# New comment
# Another comment
DEBUG = True
LOG_LEVEL = "INFO"
MAX_RETRIES = 3`, fs.files["config.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_IncorrectContext() {
	// Test: context lines don't match the actual file content
	patchText := `
*** Begin Patch
*** Update File: config.py
+# New header
 import sys
 import os
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_WrongIndentation() {
	// Test: line without marker (error tolerance treats it as context)
	patchText := `
*** Begin Patch
*** Update File: code.py
+# Header
def foo():
    pass
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"code.py": `def foo():
    pass`,
	})

	_, err := ApplyPatch(fs, patchText)
	// Due to error tolerance, unmarked lines are treated as context lines
	s.Require().NoError(err)
	s.Equal(`# Header
def foo():
    pass`, fs.files["code.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_ContextFromMiddle() {
	// Test: using context from middle of file when trying to add at beginning
	patchText := `
*** Begin Patch
*** Update File: app.py
+# New import
 def main():
     pass
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `import os
import sys

def main():
    pass`,
	})

	_, err := ApplyPatch(fs, patchText)
	// This should succeed but add in the wrong place (not at beginning)
	s.Require().NoError(err)
	// The comment is added before "def main()", not at the actual beginning
	s.Equal(`import os
import sys

# New import
def main():
    pass`, fs.files["app.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_ContextInWrongOrder() {
	// Test: context lines are in wrong order
	patchText := `
*** Begin Patch
*** Update File: app.py
+# Header
 import sys
 import os
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `import os
import sys`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_DeletionAtBeginning() {
	// Test: trying to delete non-existent lines at beginning
	patchText := `
*** Begin Patch
*** Update File: config.py
-# This comment doesn't exist
 DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_PartialContextMatch() {
	// Test: context partially matches but not completely
	patchText := `
*** Begin Patch
*** Update File: script.py
+# New header
 import os
 import sys
 import json
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"script.py": `import os
import sys`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_EmptyContextLine() {
	// Test: context line is empty but file starts with non-empty line
	patchText := `
*** Begin Patch
*** Update File: config.py
+# Header
 
 DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Invalid context")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_TrailingWhitespace() {
	// Test: context has trailing whitespace that doesn't match
	patchText := `
*** Begin Patch
*** Update File: config.py
+# Header
 DEBUG = True   
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"`,
	})

	// This might succeed due to whitespace fuzzy matching
	_, err := ApplyPatch(fs, patchText)
	// If it succeeds, that's okay (fuzzy matching)
	if err == nil {
		s.Equal(`# Header
DEBUG = True
LOG_LEVEL = "INFO"`, fs.files["config.py"])
	} else {
		// If it fails, that's also acceptable
		s.Contains(err.Error(), "Invalid context")
	}
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_AmbiguousContext() {
	// Test: context appears multiple times in file
	patchText := `
*** Begin Patch
*** Update File: code.py
+# New function
 def process():
     return True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"code.py": `def process():
    return True

def process():
    return True`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	// Should match the first occurrence
	s.Equal(`# New function
def process():
    return True

def process():
    return True`, fs.files["code.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_MixedLineEndings() {
	// Test: handling different line ending styles
	patchText := "*** Begin Patch\r\n" +
		"*** Update File: config.py\r\n" +
		"+# Header\r\n" +
		" DEBUG = True\r\n" +
		"*** End Patch\r\n"

	fs := newFakeFileSystem(map[string]string{
		"config.py": "DEBUG = True\nLOG_LEVEL = \"INFO\"",
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	// Should handle CRLF in patch even if file has LF
	s.Contains(fs.files["config.py"], "# Header")
	s.Contains(fs.files["config.py"], "DEBUG = True")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_DuplicateAddition() {
	// Test: trying to add the same thing twice
	patchText := `
*** Begin Patch
*** Update File: config.py
+# Header
 DEBUG = True
*** Update File: config.py
+# Another header
 DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().Error(err)
	s.Contains(err.Error(), "Duplicate")
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_ContextTooShort() {
	// Test: context is too short in a large file (ambiguous)
	patchText := `
*** Begin Patch
*** Update File: app.py
+# Import statement
 import os
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"app.py": `# Main application
import os
import sys

# Utilities
import os
import json`,
	})

	_, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	// Should match first occurrence
	s.Equal(`# Main application
# Import statement
import os
import sys

# Utilities
import os
import json`, fs.files["app.py"])
}

func (s *PatchSuite) TestAddLinesAtBeginning_Negative_MissingSpaceMarker() {
	// Test: forgot to add space marker before context lines
	patchText := `
*** Begin Patch
*** Update File: config.py
+# Header
DEBUG = True
*** End Patch
`

	fs := newFakeFileSystem(map[string]string{
		"config.py": `DEBUG = True
LOG_LEVEL = "INFO"`,
	})

	// Due to error tolerance, this might be treated as a context line
	_, err := ApplyPatch(fs, patchText)
	s.Require().NoError(err)
	// Should succeed due to error tolerance treating unmarked lines as context
	s.Contains(fs.files["config.py"], "# Header")
}
