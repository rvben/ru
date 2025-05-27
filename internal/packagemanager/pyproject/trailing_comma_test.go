package pyproject

import (
	"os"
	"strings"
	"testing"
)

func TestTrailingCommaPreservation(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "trailing-comma-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		inputContent   string
		expectedCommas bool
		description    string
	}{
		{
			name: "File with trailing commas",
			inputContent: `[project]
name = "test"
dependencies = [
    "requests==2.25.0",
    "flask==1.1.0",
    "django==3.0.0",
]`,
			expectedCommas: true,
			description:    "Should preserve trailing commas when they exist",
		},
		{
			name: "File without trailing commas",
			inputContent: `[project]
name = "test"
dependencies = [
    "requests==2.25.0",
    "flask==1.1.0",
    "django==3.0.0"
]`,
			expectedCommas: true,
			description:    "Should add trailing commas for multi-line arrays (uv standard)",
		},
		{
			name: "Single line array with trailing comma",
			inputContent: `[project]
name = "test"
dependencies = ["requests==2.25.0",]`,
			expectedCommas: true,
			description:    "Should preserve trailing comma in single-line arrays",
		},
		{
			name: "Single line array without trailing comma",
			inputContent: `[project]
name = "test"
dependencies = ["requests==2.25.0"]`,
			expectedCommas: false,
			description:    "Should NOT add trailing comma to single-line arrays",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := tempDir + "/test.toml"
			err := os.WriteFile(filePath, []byte(tt.inputContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create PyProject and update it
			pyproject := NewPyProject(filePath)
			versions := map[string]string{
				"requests": "2.32.3",
				"flask":    "3.1.1",
				"django":   "5.2.1",
			}

			_, err = pyproject.LoadAndUpdate(versions)
			if err != nil {
				t.Fatalf("LoadAndUpdate failed: %v", err)
			}

			// Read the updated content
			updatedContent, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("Failed to read updated file: %v", err)
			}

			updatedStr := string(updatedContent)

			// Check if trailing commas are preserved/not added
			if tt.expectedCommas {
				// Should have trailing comma on the last dependency
				// Determine which is the last dependency based on the input
				var lastDependencyPattern string
				if strings.Contains(tt.inputContent, "django") {
					lastDependencyPattern = `"django==5.2.1",`
				} else {
					lastDependencyPattern = `"requests==2.32.3",`
				}

				if !strings.Contains(updatedStr, lastDependencyPattern) {
					t.Errorf("%s: Expected trailing comma to be preserved, but it was removed", tt.description)
					t.Logf("Updated content:\n%s", updatedStr)
				}
			} else {
				// Should NOT have trailing comma on the last dependency
				// Determine which is the last dependency based on the input
				var lastDependencyPattern string
				if strings.Contains(tt.inputContent, "django") {
					lastDependencyPattern = `"django==5.2.1",`
				} else {
					lastDependencyPattern = `"requests==2.32.3",`
				}

				if strings.Contains(updatedStr, lastDependencyPattern) {
					t.Errorf("%s: Expected NO trailing comma, but one was added", tt.description)
					t.Logf("Updated content:\n%s", updatedStr)
				}
			}

			// Verify versions were updated
			if !strings.Contains(updatedStr, "2.32.3") {
				t.Errorf("Versions were not updated correctly")
			}
		})
	}
}
