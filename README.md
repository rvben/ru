# RU (Requirements Updater)

A tool for updating Python and Node.js package dependencies. RU automatically updates your requirements files to the latest package versions while respecting versioning constraints.

## Features

- Updates Python requirements.txt files
- Updates Node.js package.json files
- Updates Poetry pyproject.toml files
- Supports custom PyPI indexes (multiple ways)
- Version caching for performance
- Dependency verification (optional)
- Self-update functionality
- Gitignore-aware file processing

## Installation

```bash
# Install from binary releases
curl -s https://raw.githubusercontent.com/rvben/ru/main/install.sh | bash

# Or build from source
go build -o ru ./cmd/ru
```

## Usage

```bash
# Update all dependencies in the current directory and subdirectories
ru update

# Update with dependency verification (slower)
ru update --verify

# Update with verbose logging
ru update --verbose

# Update without caching
ru update --no-cache

# Show version information
ru version

# Clean the version cache
ru clean-cache

# Update ru itself to the latest version
ru self update
```

## Custom Package Index Support

RU supports custom package indexes from multiple sources with the following precedence:

1. **Environment Variables**
   - `UV_INDEX_URL`: Used by uv package installer
   - `PIP_INDEX_URL`: Used by pip package installer
   - `PYTHON_INDEX_URL`: Generic variable

2. **Requirements Files**
   - Using `--index-url` directive in requirements.txt:
     ```
     --index-url https://my-custom-index.example.com
     flask==2.0.0
     requests==2.28.0
     ```
   - Using `-i` shorthand:
     ```
     -i https://my-custom-index.example.com
     flask==2.0.0
     requests==2.28.0
     ```

3. **pyproject.toml Files**
   - Using UV index configuration:
     ```toml
     [[tool.uv.index]]
     name = "custom"
     url = "https://my-custom-index.example.com"
     default = true  # Makes this the default source
     
     [[tool.uv.index]]
     name = "secondary"
     url = "https://secondary-index.example.com"
     ```
   - Using Poetry source configuration:
     ```toml
     [tool.poetry.source]
     name = "custom"
     url = "https://my-custom-index.example.com"
     ```
   - Using pip configuration:
     ```toml
     [tool.pip]
     index-url = "https://my-custom-index.example.com"
     ```

4. **pip.conf File**
   - Located at `~/.config/pip/pip.conf`, `~/.pip/pip.conf`, or `/etc/pip.conf`:
     ```ini
     [global]
     index-url = https://my-custom-index.example.com
     ```

RU will automatically detect and use the appropriate custom index URL based on this order of precedence.

## File Patterns Supported

### Python Requirements Files
- `requirements.txt`
- `requirements-*.txt`
- `requirements_*.txt`
- `*.requirements.txt`
- `requirements-dev.txt`
- `requirements_dev.txt`

### Node.js Files
- `package.json`

### Poetry Files
- `pyproject.toml`

## Version Handling

- Supports semantic versioning
- Handles pre-release versions
- Respects version constraints (==, >=, <=, ~=, etc.)
- Preserves existing constraints when updating

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.