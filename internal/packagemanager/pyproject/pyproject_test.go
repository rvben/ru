package pyproject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndUpdate(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		versions map[string]string
		want     string
	}{
		{
			name: "PEP 621 format",
			content: `[project]
name = "test-project"
version = "0.1.0"
dependencies = [
    "requests==2.25.1",
    "flask>=2.0.0,<3.0.0",
    "pytest==6.2.5",
]

[project.optional-dependencies]
dev = [
    "black==22.3.0",
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
		},
		{
			name: "Complete PEP 621 format with dependency groups",
			content: `[project]
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
    "pytest==6.2.5",
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
		},
		{
			name: "Poetry format",
			content: `[tool.poetry]
dependencies = { requests = "^2.25.1", flask = ">=2.0.0,<3.0.0" }
dev-dependencies = { pytest = "^6.2.5" }`,
			versions: map[string]string{
				"requests": "2.31.0",
				"pytest":   "7.4.3",
			},
			want: `[tool.poetry]
dependencies = { flask = ">=2.0.0,<3.0.0", requests = "^2.31.0" }
dev-dependencies = { pytest = "^7.4.3" }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "pyproject.toml")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Run the update
			if err := LoadAndUpdate(tmpFile, tt.versions); err != nil {
				t.Fatalf("LoadAndUpdate() error = %v", err)
			}

			// Read the result
			got, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatalf("Failed to read updated file: %v", err)
			}

			// Compare results (normalize line endings)
			gotStr := strings.TrimSpace(strings.ReplaceAll(string(got), "\r\n", "\n"))
			wantStr := strings.TrimSpace(strings.ReplaceAll(tt.want, "\r\n", "\n"))

			// Print debug info if test fails
			if gotStr != wantStr {
				t.Errorf("LoadAndUpdate() produced incorrect output\nwant (len=%d):\n%q\ngot (len=%d):\n%q",
					len(wantStr), wantStr, len(gotStr), gotStr)
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
