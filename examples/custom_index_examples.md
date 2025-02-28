# Custom PyPI Index URL Examples

This document provides examples of different ways to specify custom PyPI index URLs when using the RU tool.

## Option 1: Environment Variables

Environment variables are checked first and have the highest precedence.

```bash
# Using uv's environment variable (highest precedence)
export UV_INDEX_URL="https://custom-pypi.example.com/simple"
ru update

# Using pip's environment variable (second highest precedence)
export PIP_INDEX_URL="https://custom-pypi.example.com/simple"
ru update

# Using the generic environment variable
export PYTHON_INDEX_URL="https://custom-pypi.example.com/simple"
ru update
```

## Option 2: In Requirements Files

You can specify a custom index URL directly in your requirements files.

### Example requirements.txt with index URL

```
--index-url https://custom-pypi.example.com/simple
flask==2.0.1
requests==2.26.0
```

### Using the shorthand notation

```
-i https://custom-pypi.example.com/simple
flask==2.0.1
requests==2.26.0
```

## Option 3: In pyproject.toml

You can specify a custom index URL in your pyproject.toml file using either Poetry's format or pip's format.

### Poetry Format

```toml
[tool.poetry.source]
name = "custom"
url = "https://custom-pypi.example.com/simple"
priority = "primary"  # Optional, "primary" makes this the default source

[tool.poetry.dependencies]
flask = "^2.0.1"
requests = "^2.26.0"
```

### pip Format

```toml
[tool.pip]
index-url = "https://custom-pypi.example.com/simple"

[project]
dependencies = [
    "flask>=2.0.1",
    "requests>=2.26.0"
]
```

## Option 4: Using pip.conf

You can create a pip.conf file in one of the standard locations:

- `~/.config/pip/pip.conf` (Unix/macOS)
- `~/.pip/pip.conf` (Alternative Unix/macOS)
- `/etc/pip.conf` (System-wide)

### Example pip.conf

```ini
[global]
index-url = https://custom-pypi.example.com/simple
```

## AWS CodeArtifact Example

If you're using AWS CodeArtifact, RU will automatically detect the URL format and make the necessary adjustments.

```bash
# Using AWS CodeArtifact URL
export PIP_INDEX_URL="https://domain-1234567890.d.codeartifact.us-west-2.amazonaws.com/pypi/repo/simple/"
ru update
```

## Special URL Handling

- URLs that already end with `/simple` will be used as-is
- AWS CodeArtifact URLs are automatically detected and properly formatted
- For other URLs, RU will append `/simple` if needed to follow the PEP 503 convention

## Using Multiple Sources (Poetry only)

Poetry allows you to specify multiple sources in your pyproject.toml:

```toml
[[tool.poetry.source]]
name = "custom-primary"
url = "https://custom-primary.example.com/simple"
priority = "primary"

[[tool.poetry.source]]
name = "custom-secondary"
url = "https://custom-secondary.example.com/simple"
priority = "supplemental"
```

RU will use the source marked as "primary" when fetching package versions.

## Notes

- The order of precedence is: Environment Variables → Requirements File → pyproject.toml → pip.conf
- When multiple custom index URLs are found from different sources, the highest precedence one is used
- All custom index URLs are validated before use 