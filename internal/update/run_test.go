package update

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rvben/ru/internal/packagemanager/npm"
)

// MockRunPyPI implements the PackageManager interface for testing Run method
type MockRunPyPI struct {
	versions map[string]string
}

func (m *MockRunPyPI) GetLatestVersion(pkg string) (string, error) {
	if version, ok := m.versions[pkg]; ok {
		return version, nil
	}
	return "", fmt.Errorf("package %s not found", pkg)
}

func (m *MockRunPyPI) SetCustomIndexURL() error {
	return nil
}

// TestRunOutput tests the output message formatting in the Run method
func TestRunOutput(t *testing.T) {
	tests := []struct {
		name             string
		requirements     string
		versions         map[string]string
		expectedMessage  string
		expectedFiles    int
		expectedPackages int
	}{
		{
			name: "Single file single package",
			requirements: `package1==1.0.0
`,
			versions: map[string]string{
				"package1": "2.0.0",
			},
			expectedMessage:  "1 file updated with 1 package upgraded",
			expectedFiles:    1,
			expectedPackages: 1,
		},
		{
			name: "Single file multiple packages",
			requirements: `package1==1.0.0
package2==1.0.0
package3==1.0.0
`,
			versions: map[string]string{
				"package1": "2.0.0",
				"package2": "2.0.0",
				"package3": "2.0.0",
			},
			expectedMessage:  "1 file updated with 3 packages upgraded",
			expectedFiles:    1,
			expectedPackages: 3,
		},
		{
			name: "No updates needed",
			requirements: `package1==1.0.0
`,
			versions: map[string]string{
				"package1": "1.0.0", // Same version, no update needed
			},
			expectedMessage:  "No updates were made",
			expectedFiles:    0,
			expectedPackages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup a temporary directory for the test
			tmpDir, err := os.MkdirTemp("", "test-run-output")
			if err != nil {
				t.Fatalf("Failed to create temp directory: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Change to the temp directory for the test
			currentDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get current directory: %v", err)
			}
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("Failed to change to temp directory: %v", err)
			}
			defer os.Chdir(currentDir)

			// Create a test requirements file
			testFile := "requirements.txt"
			err = os.WriteFile(testFile, []byte(tt.requirements), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Redirect stdout to capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create an updater with test settings and mock PyPI
			updater := &Updater{
				pypi:           &MockRunPyPI{versions: tt.versions},
				npm:            npm.New(),
				filesUpdated:   0,
				filesUnchanged: 0,
				modulesUpdated: 0,
				paths:          []string{"."},
				verify:         false,
			}

			// Run the update
			err = updater.Run()
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			// Close the write end of the pipe and restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read the captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := strings.TrimSpace(buf.String())

			// Check if the output contains the expected message
			if !strings.Contains(output, tt.expectedMessage) {
				t.Errorf("Output message incorrect.\nExpected to contain: %s\nGot: %s",
					tt.expectedMessage, output)
			}

			// Verify the counts are as expected
			if tt.expectedFiles > 0 && updater.filesUpdated != tt.expectedFiles {
				t.Errorf("Expected %d files updated, got %d", tt.expectedFiles, updater.filesUpdated)
			}
			if tt.expectedPackages > 0 && updater.modulesUpdated != tt.expectedPackages {
				t.Errorf("Expected %d packages updated, got %d", tt.expectedPackages, updater.modulesUpdated)
			}
		})
	}
}
