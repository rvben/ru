#!/bin/bash
# Demo script showing different ways to use custom index URLs with RU

# Function to print section headers
print_header() {
    echo
    echo "====================================================="
    echo "  $1"
    echo "====================================================="
    echo
}

# Function to show current index URL configuration
show_index_config() {
    echo "Current environment variables:"
    echo "  UV_INDEX_URL: ${UV_INDEX_URL:-not set}"
    echo "  PIP_INDEX_URL: ${PIP_INDEX_URL:-not set}"
    echo "  PYTHON_INDEX_URL: ${PYTHON_INDEX_URL:-not set}"
    echo
}

# Clean up environment variables at the end
cleanup() {
    print_header "Cleaning up environment variables"
    unset UV_INDEX_URL
    unset PIP_INDEX_URL
    unset PYTHON_INDEX_URL
    echo "Environment variables unset"
}

# Set trap to ensure cleanup even if script is interrupted
trap cleanup EXIT

# Start the demo
print_header "RU Custom Index URL Demo"
echo "This script demonstrates the different ways to specify a custom PyPI index URL with RU."
echo "Note: Commands are shown for demonstration purposes and won't actually run packages."

# Default configuration
print_header "Default Configuration"
show_index_config
echo "When no custom index URL is specified, RU will use the default PyPI:"
echo "  $ ru update"

# Using environment variables
print_header "1. Using Environment Variables"

echo "Setting UV_INDEX_URL environment variable (highest precedence):"
export UV_INDEX_URL="https://uv-index.example.com/simple"
show_index_config
echo "  $ ru update"
echo "RU will use: https://uv-index.example.com/simple"
echo

echo "Setting PIP_INDEX_URL environment variable:"
unset UV_INDEX_URL
export PIP_INDEX_URL="https://pip-index.example.com/simple"
show_index_config
echo "  $ ru update"
echo "RU will use: https://pip-index.example.com/simple"
echo

echo "When both are set, UV_INDEX_URL takes precedence:"
export UV_INDEX_URL="https://uv-index.example.com/simple"
show_index_config
echo "  $ ru update"
echo "RU will use: https://uv-index.example.com/simple"

# Using requirements.txt
print_header "2. Using requirements.txt"
unset UV_INDEX_URL
unset PIP_INDEX_URL
show_index_config

echo "With a requirements.txt file containing:"
echo "  --index-url https://req-file-index.example.com/simple"
echo "  flask==2.0.1"
echo "  requests==2.26.0"
echo
echo "  $ ru update"
echo "RU will use: https://req-file-index.example.com/simple"

# Using pyproject.toml
print_header "3. Using pyproject.toml"
show_index_config

echo "With a pyproject.toml file containing Poetry configuration:"
echo "  [tool.poetry.source]"
echo "  name = \"custom\""
echo "  url = \"https://poetry-index.example.com/simple\""
echo "  priority = \"primary\""
echo
echo "  $ ru update"
echo "RU will use: https://poetry-index.example.com/simple"
echo

echo "With a pyproject.toml file containing pip configuration:"
echo "  [tool.pip]"
echo "  index-url = \"https://pip-config-index.example.com/simple\""
echo
echo "  $ ru update"
echo "RU will use: https://pip-config-index.example.com/simple"

# Order of precedence
print_header "4. Order of Precedence"
export UV_INDEX_URL="https://uv-index.example.com/simple"
show_index_config

echo "With environment variables set AND requirements.txt AND pyproject.toml configured:"
echo "1. Environment variables (highest precedence)"
echo "2. requirements.txt"
echo "3. pyproject.toml"
echo "4. pip.conf"
echo
echo "RU will use: https://uv-index.example.com/simple (from UV_INDEX_URL)"

print_header "Demo Complete"
echo "See examples directory for more detailed configuration examples:"
echo "  - custom_index_examples.md      - Comprehensive documentation"
echo "  - requirements_with_index.txt   - Example requirements file"
echo "  - pyproject_with_poetry_index.toml - Example Poetry configuration"
echo "  - pyproject_with_pip_index.toml - Example pip configuration"
echo "  - pip.conf.example              - Example pip.conf file" 