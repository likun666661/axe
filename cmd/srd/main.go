package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stumble/axe/code/srd"
)

// FileSystemImpl implements the srd.FileSystem interface
type FileSystemImpl struct {
	baseDir string
}

func (fs *FileSystemImpl) Open(path string) (string, error) {
	fullPath := filepath.Join(fs.baseDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (fs *FileSystemImpl) Write(path string, content string) error {
	fullPath := filepath.Join(fs.baseDir, path)

	// Create parent directories if they don't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return os.WriteFile(fullPath, []byte(content), 0600)
}

func (fs *FileSystemImpl) Remove(path string) error {
	fullPath := filepath.Join(fs.baseDir, path)
	return os.Remove(fullPath)
}

func main() {
	// Define flags
	patchFile := flag.String("patch", "", "Path to the patch/diff file (required)")
	targetDir := flag.String("dir", ".", "Target directory to apply the patch (default: current directory)")
	help := flag.Bool("help", false, "Show help message")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: srd -patch <patch_file> [-dir <target_directory>]\n\n")
		fmt.Fprintf(os.Stderr, "Apply a patch file to a directory using the SRD (Search-Replace-Delete) format.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  srd -patch changes.diff -dir /path/to/project\n")
		fmt.Fprintf(os.Stderr, "  srd -patch changes.diff  # applies to current directory\n")
	}

	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	// Validate required flags
	if *patchFile == "" {
		fmt.Fprintf(os.Stderr, "Error: -patch flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Read the patch file
	patchContent, err := os.ReadFile(*patchFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading patch file: %v\n", err)
		os.Exit(1)
	}

	// Convert target directory to absolute path
	absDir, err := filepath.Abs(*targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving target directory: %v\n", err)
		os.Exit(1)
	}

	// Check if target directory exists
	if stat, err := os.Stat(absDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: target directory does not exist: %v\n", err)
		os.Exit(1)
	} else if !stat.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: target path is not a directory: %s\n", absDir)
		os.Exit(1)
	}

	// Create filesystem implementation
	fs := &FileSystemImpl{baseDir: absDir}

	// Apply the patch
	fmt.Printf("Applying patch from '%s' to directory '%s'...\n", *patchFile, absDir)
	result, err := srd.ApplyPatch(fs, string(patchContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error applying patch: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Success: %s\n", result)
}
