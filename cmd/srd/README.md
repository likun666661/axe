# SRD Command-Line Tool

A simple command-line tool to apply SRD (Search-Replace-Delete) format patch files to directories.

## Installation

```bash
cd cmd/srd
go build -o srd
```

Or install globally:

```bash
go install github.com/yxia/axe/cmd/srd@latest
```

## Usage

```bash
srd -patch <patch_file> [-dir <target_directory>]
```

### Options

- `-patch <file>` (required): Path to the patch file to apply
- `-dir <directory>` (optional): Target directory to apply the patch (defaults to current directory)
- `-help`: Show help message

### Examples

Apply a patch to the current directory:
```bash
srd -patch changes.diff
```

Apply a patch to a specific directory:
```bash
srd -patch changes.diff -dir /path/to/project
```

Apply the example patch:
```bash
cd /home/yxia/stumble/axe
srd -patch cmd/examples/great_test_writer/failed_diff.txt -dir cmd/examples/great_test_writer/
```

## Patch Format

The tool uses the SRD patch format. See `/code/srd/README.md` for detailed documentation on how to write patches.

Quick example:

```
*** Begin Patch
*** Update File: src/hello.py
 def greet(name):
-    print("Hello")
+    print("Hello, World!")
*** End Patch
```

## Features

- Add new files
- Update existing files
- Delete files
- Move/rename files
- Automatic directory creation for new files

