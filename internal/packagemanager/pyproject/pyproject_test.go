package pyproject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndUpdate(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		versions       map[string]string
		expectedError  bool
		expectedOutput string
	}{
		{
			name: "basic dependencies",
			content: `[project]
dependencies = [
    "requests==2.31.0",
    "flask==2.0.0"
]`,
			versions: map[string]string{
				"requests": "2.32.0",
				"flask":    "3.0.0",
			},
			expectedOutput: `[project]
dependencies = [
    "requests==2.32.0",
    "flask==3.0.0"
]`,
		},
		{
			name: "dependency groups",
			content: `[dependency-groups]
test = [
    "pytest==7.0.0",
    "coverage==6.0.0"
]
dev = [
    "black==22.0.0",
    { include-group = "test" }
]

[project]
dependencies = [
    "requests==2.31.0"
]`,
			versions: map[string]string{
				"pytest":   "7.4.0",
				"coverage": "7.3.0",
				"black":    "23.9.0",
				"requests": "2.32.0",
			},
			expectedOutput: `[dependency-groups]
test = [
    "pytest==7.4.0",
    "coverage==7.3.0"
]
dev = [
    "black==23.9.0",
    { include-group = "test" }
]

[project]
dependencies = [
    "requests==2.32.0"
]`,
		},
		{
			name: "optional dependencies",
			content: `[project]
dependencies = [
    "requests==2.31.0"
]
[project.optional-dependencies]
test = [
    "pytest==7.0.0",
    "coverage==6.0.0"
]`,
			versions: map[string]string{
				"requests": "2.32.0",
				"pytest":   "7.4.0",
				"coverage": "7.3.0",
			},
			expectedOutput: `[project]
dependencies = [
    "requests==2.32.0"
]
[project.optional-dependencies]
test = [
    "pytest==7.4.0",
    "coverage==7.3.0"
]`,
		},
		{
			name: "invalid toml",
			content: `[project
dependencies = [
    "requests==2.31.0"
]`,
			versions: map[string]string{
				"requests": "2.32.0",
			},
			expectedError: true,
		},
		{
			name: "invalid dependency group",
			content: `[dependency-groups]
test = [
    "pytest==7.0.0",
    { invalid = "value" }
]`,
			versions: map[string]string{
				"pytest": "7.4.0",
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "pyproject-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create test file
			testFile := filepath.Join(tmpDir, "pyproject.toml")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Run the update
			err = LoadAndUpdate(testFile, tt.versions)

			// Check error expectation
			if tt.expectedError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// If we expect success, verify the output
			if !tt.expectedError {
				output, err := os.ReadFile(testFile)
				if err != nil {
					t.Fatalf("Failed to read output file: %v", err)
				}

				// Normalize line endings
				got := string(output)
				got = strings.ReplaceAll(got, "\r\n", "\n")
				expected := strings.ReplaceAll(tt.expectedOutput, "\r\n", "\n")

				if got != expected {
					t.Errorf("Output mismatch:\nGot:\n%s\nWant:\n%s", got, expected)
				}
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
			got, changed := updateDependencyVersion(tt.dep, tt.versions)
			if got != tt.want {
				t.Errorf("updateDependencyVersion() got = %v, want %v", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("updateDependencyVersion() changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}
