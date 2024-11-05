# ru - Python Dependency Manager

`ru` is a tool for managing Python dependencies. It can update your dependencies to their latest versions, align versions across multiple files, and more.

## Quick Start

``` bash
# Install ru
go install github.com/rvben/ru/cmd/ru@latest

# Update all dependencies to latest versions
ru update

# Align versions across all files
ru align
```

## Commands

### `ru update`
Updates all dependencies to their latest versions in:
- requirements.txt files
- pyproject.toml files (PEP 735 compatible)
- package.json files

``` bash
# Update with verbose logging
ru update -verbose

# Update without using cache
ru update -no-cache
```

### `ru align`
Aligns package versions across all files. Uses the highest version found in your codebase for each package.

``` bash
# Align versions with verbose logging
ru align -verbose
```

## Configuration

### Ignoring Updates
You can prevent specific packages from being updated by adding them to your `pyproject.toml`:

``` toml
[tool.ru]
ignore-updates = [
    "flask",  # Never update flask
    "requests"  # Never update requests
]
```

### Custom Package Index
`ru` automatically uses your custom PyPI index from `pip.conf`. Supported locations:
- `~/.config/pip/pip.conf`
- `/etc/pip.conf`

Example pip.conf:
``` ini
[global]
index-url = https://your-custom-pypi.example.com/simple
```

### Supported File Types
- `requirements.txt`: Python requirements files
- `pyproject.toml`: Python project files (PEP 735 compatible)
  - Regular dependencies
  - Optional dependencies
  - Dependency groups
- `package.json`: Node.js package files

### .gitignore Support
`ru` respects your `.gitignore` patterns and won't process files in ignored directories (like `.venv`, `node_modules`, etc.).

## Additional Commands

### `ru self-update`
Updates ru to the latest version:
``` bash
ru self-update
```

### `ru clean-cache`
Clears the version cache:
``` bash
ru clean-cache
```

### `ru version`
Shows the current version:
``` bash
ru version
```

## Flags

- `-verbose`: Enable detailed logging
- `-no-cache`: Disable caching of package versions

## Error Handling

The tool provides clear error messages for common issues:
- Custom index not reachable
- Authentication failures
- Invalid version formats
- Package not found

## Examples

### Basic Update
``` bash
$ ru update
2 files updated and 5 modules updated
```

### Version Alignment
``` bash
$ ru align
3 files aligned and 7 modules updated
```

### pyproject.toml Example
``` toml
[project]
dependencies = [
    "requests==2.31.0",
    "flask==2.0.0"
]

[project.optional-dependencies]
test = [
    "pytest==7.0.0",
    "coverage==6.0.0"
]

[dependency-groups]
test = [
    "pytest==7.0.0",
    "coverage==6.0.0"
]
dev = [
    "black==22.0.0",
    { include-group = "test" }
]

[tool.ru]
ignore-updates = [
    "flask"  # Keep flask at its current version
]
```

## License

MIT License - see LICENSE file for details