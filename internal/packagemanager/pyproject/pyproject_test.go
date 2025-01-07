package pyproject

import (
	"os"
	"strings"
	"testing"
)

func TestLoadAndUpdate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		versions map[string]string
		want     string
		wantErr  bool
		changed  bool
	}{
		{
			name: "PEP 621 format",
			input: `[project]
name = "test-project"
version = "0.1.0"
dependencies = [
    "requests==2.25.1",
    "flask>=2.0.0,<3.0.0",
    "pytest==6.2.5",
]

[project.optional-dependencies]
dev = [
    "black==23.12.0"
]`,
			versions: map[string]string{
				"requests": "2.31.0",
				"pytest":   "7.4.3",
				"black":    "23.12.1",
			},
			want: `[project]
name = "test-project"
version = "0.1.0"
dependencies = [
    "flask>=2.0.0,<3.0.0",
    "pytest==7.4.3",
    "requests==2.31.0"
]

[project.optional-dependencies]
dev = [
    "black==23.12.1"
]`,
			wantErr: false,
			changed: true,
		},
		{
			name: "Complete PEP 621 format with dependency groups",
			input: `[project]
name = "example-project"
version = "0.1.0"
description = "Add your description here"
readme = "README.md"
requires-python = ">=3.13"
dependencies = [
    "aws-cdk-lib==2.164.1",
    "constructs>=10.0.0,<11.0.0",
]

[dependency-groups]
dev = [
    "pytest==7.4.2"
]`,
			versions: map[string]string{
				"aws-cdk-lib": "2.165.0",
				"pytest":      "7.4.3",
			},
			want: `[project]
name = "example-project"
version = "0.1.0"
description = "Add your description here"
readme = "README.md"
requires-python = ">=3.13"
dependencies = [
    "aws-cdk-lib==2.165.0",
    "constructs>=10.0.0,<11.0.0"
]

[dependency-groups]
dev = [
    "pytest==7.4.3"
]`,
			wantErr: false,
			changed: true,
		},
		{
			name: "Poetry format",
			input: `[tool.poetry]
dependencies = { flask = ">=2.0.0,<3.0.0", requests = "^2.25.1" }
dev-dependencies = { pytest = "^6.2.5" }`,
			versions: map[string]string{
				"requests": "2.31.0",
				"pytest":   "7.4.3",
			},
			want: `[tool.poetry]
dependencies = { flask = ">=2.0.0,<3.0.0", requests = "^2.31.0" }
dev-dependencies = { pytest = "^7.4.3" }`,
			wantErr: false,
			changed: true,
		},
		{
			name: "Preserve non-dependency sections",
			input: `[project]
name = "example-project"
version = "0.1.0"
dependencies = [
    "flask==2.0.0",
    "requests==2.25.1"
]

[tool.isort]
profile = "black"

[tool.pylint.format]
max-line-length = "120"

[tool.ruff]
line-length = 120

[tool.ruff.lint]
ignore = ["E501"]

[tool.ruff.format]
quote-style = "double"
indent-style = "space"
skip-magic-trailing-comma = false
line-ending = "auto"

[tool.bandit]
exclude_dirs = ["tests"]
exclude = ["*_test.py", "test_*.py"]
skips = ["B101","B405","B608"]

[tool.bandit.assert_used]
exclude = ["*_test.py", "test_*.py"]

[tool.setuptools]
py-modules = []

[tool.sqlfluff.core]
sql_file_exts = ".sql"
max_line_length = 160
exclude_rules = "RF04"

[tool.coverage.run]
branch = true
relative_files = true
source = ['.']
omit = [
    'cdk.out',
    '**/.venv/*',
    'tests/*',
    '*/test_*.py',
]

[tool.coverage.report]
include_namespace_packages = true
omit = [
    '**/__init__.py',
    'cdk.out/*'
]
skip_empty = true`,
			versions: map[string]string{
				"flask":    "2.3.3",
				"requests": "2.31.0",
			},
			want: `[project]
name = "example-project"
version = "0.1.0"
dependencies = [
    "flask==2.3.3",
    "requests==2.31.0"
]

[tool.isort]
profile = "black"

[tool.pylint.format]
max-line-length = "120"

[tool.ruff]
line-length = 120

[tool.ruff.lint]
ignore = ["E501"]

[tool.ruff.format]
quote-style = "double"
indent-style = "space"
skip-magic-trailing-comma = false
line-ending = "auto"

[tool.bandit]
exclude_dirs = ["tests"]
exclude = ["*_test.py", "test_*.py"]
skips = ["B101","B405","B608"]

[tool.bandit.assert_used]
exclude = ["*_test.py", "test_*.py"]

[tool.setuptools]
py-modules = []

[tool.sqlfluff.core]
sql_file_exts = ".sql"
max_line_length = 160
exclude_rules = "RF04"

[tool.coverage.run]
branch = true
relative_files = true
source = ['.']
omit = [
    'cdk.out',
    '**/.venv/*',
    'tests/*',
    '*/test_*.py',
]

[tool.coverage.report]
include_namespace_packages = true
omit = [
    '**/__init__.py',
    'cdk.out/*'
]
skip_empty = true`,
			wantErr: false,
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file
			tmpFile, err := os.CreateTemp("", "pyproject.toml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())

			// Write the input content
			if err := os.WriteFile(tmpFile.Name(), []byte(tt.input), 0644); err != nil {
				t.Fatal(err)
			}

			// Run LoadAndUpdate
			changed, err := LoadAndUpdate(tmpFile.Name(), tt.versions)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadAndUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if changed != tt.changed {
				t.Errorf("LoadAndUpdate() changed = %v, want %v", changed, tt.changed)
			}

			// Read the output
			got, err := os.ReadFile(tmpFile.Name())
			if err != nil {
				t.Fatal(err)
			}

			// Normalize line endings and trailing whitespace
			gotStr := strings.TrimSpace(strings.ReplaceAll(string(got), "\r\n", "\n"))
			wantStr := strings.TrimSpace(strings.ReplaceAll(tt.want, "\r\n", "\n"))

			if gotStr != wantStr {
				t.Errorf("LoadAndUpdate() produced incorrect output\nwant (len=%d):\n%s\ngot (len=%d):\n%s", len(wantStr), wantStr, len(gotStr), gotStr)
			}
		})
	}
}

func TestUpdateDependencyVersion(t *testing.T) {
	tests := []struct {
		name     string
		dep      string
		versions map[string]string
		want     string
		changed  bool
	}{
		{
			name: "basic update",
			dep:  "requests==2.31.0",
			versions: map[string]string{
				"requests": "2.32.0",
			},
			want:    "requests==2.32.0",
			changed: true,
		},
		{
			name: "no update needed",
			dep:  "requests==2.32.0",
			versions: map[string]string{
				"requests": "2.32.0",
			},
			want:    "requests==2.32.0",
			changed: false,
		},
		{
			name: "package not in versions",
			dep:  "requests==2.31.0",
			versions: map[string]string{
				"flask": "3.0.0",
			},
			want:    "requests==2.31.0",
			changed: false,
		},
		{
			name:     "empty line",
			dep:      "",
			versions: map[string]string{},
			want:     "",
			changed:  false,
		},
		{
			name:     "comment line",
			dep:      "# requests==2.31.0",
			versions: map[string]string{},
			want:     "# requests==2.31.0",
			changed:  false,
		},
		{
			name: "non-equality constraint",
			dep:  "requests>=2.31.0",
			versions: map[string]string{
				"requests": "2.32.0",
			},
			want:    "requests>=2.31.0",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateDependencyString(tt.dep, tt.versions)
			changed := got != tt.dep
			if got != tt.want {
				t.Errorf("updateDependencyString() got = %v, want %v", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("updateDependencyString() changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}
