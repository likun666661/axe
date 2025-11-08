# How to write patches

Every patch starts with `*** Begin Patch` and ends with `*** End Patch`. Between these markers, you specify what changes to make.

```
*** Begin Patch
(file operations go here)
*** End Patch
```

Before writing a patch, you should THINK DEEP on WHICH OPERATION TO USE. If the changes are small and target specific lines, use `*** Update File:`. If the changes are large and target the entire file, use `*** Add File:`.

## Operations

### Adding / Rewriting Files (When changes are large and target the entire file)

To create a new file or rewrite an existing file, use `*** Add File:` followed by the path. Prefix each line with `+`:

```
*** Add File: utils/helper.py
+def calculate(x, y):
+    return x + y
+
+def format_output(value):
+    return f"Result: {value}"
```

### Updating Files (When changes are small and target specific lines)

To modify an existing file, use `*** Update File:` followed by the file path. Then show which lines to change:

```
*** Update File: src/hello.py
 def greet(name):
-    print("Hello")
-    return "Hi"
+    print("Hello, World!")
+    return f"Hello, {name}!"
```

**The three line markers (Every line in an update section **must** start with one of these three markers):**
- **` ` (space)** = Context line (unchanged). Used to locate where the change should happen.
- **`-`** = Remove this line
- **`+`** = Add this line

**Example: Changing a line**
```
*** Update File: config.py
 DEBUG = False
-LOG_LEVEL = "INFO"
+LOG_LEVEL = "DEBUG"
 MAX_RETRIES = 3
```

### Deleting Files

To remove a file, just specify its path:

```
*** Delete File: old/deprecated.py
```

### Moving/Renaming Files

To rename or move a file while updating it, add `*** Move to:` after the update declaration:

```
*** Update File: config.py
*** Move to: settings.py
 DEBUG = False
-LOG_LEVEL = "INFO"
+LOG_LEVEL = "DEBUG"
```

This will update the file content AND move it from `config.py` to `settings.py`.

## Complete Example

Here's a patch that does multiple operations:

```
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
```

This patch:
1. Updates `main.py` to print an additional message
2. Deletes `old/deprecated.py`
3. Creates a new file `utils/helper.py`
4. Updates and moves `config.py` to `settings/config.py`
