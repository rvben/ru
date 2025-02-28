package update

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rvben/ru/internal/packagemanager/pypi"
)

// MockOutputPyPI implements the PackageManager interface for testing
type MockOutputPyPI struct {
	versions map[string]string
}

func (m *MockOutputPyPI) GetLatestVersion(pkg string) (string, error) {
	if version, ok := m.versions[pkg]; ok {
		return version, nil
	}
	return "", fmt.Errorf("package %s not found", pkg)
}

func (m *MockOutputPyPI) SetCustomIndexURL() error {
	return nil
}

func TestOutputMessageFormat(t *testing.T) {
	tests := []struct {
		name           string
		filesUpdated   int
		modulesUpdated int
		expectedOutput string
	}{
		{
			name:           "One file one package",
			filesUpdated:   1,
			modulesUpdated: 1,
			expectedOutput: "1 file updated with 1 package upgraded",
		},
		{
			name:           "One file multiple packages",
			filesUpdated:   1,
			modulesUpdated: 3,
			expectedOutput: "1 file updated with 3 packages upgraded",
		},
		{
			name:           "Multiple files one package",
			filesUpdated:   2,
			modulesUpdated: 1,
			expectedOutput: "2 files updated with 1 package upgraded",
		},
		{
			name:           "Multiple files multiple packages",
			filesUpdated:   2,
			modulesUpdated: 3,
			expectedOutput: "2 files updated with 3 packages upgraded",
		},
		{
			name:           "No updates",
			filesUpdated:   0,
			modulesUpdated: 0,
			expectedOutput: "No updates were made. All packages are already at their latest versions.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect stdout to capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create a fully initialized updater with the test values
			updater := &Updater{
				pypi:           pypi.New(true), // Use real PyPI with no-cache
				npm:            nil,            // npm can be nil for this test
				filesUpdated:   tt.filesUpdated,
				filesUnchanged: 0,
				modulesUpdated: tt.modulesUpdated,
				paths:          []string{"."},
				verify:         false,
			}

			// Mock implementation directly in this test
			updater.pypi = &MockOutputPyPI{
				versions: map[string]string{},
			}

			// Call Run which will print the output message
			err := updater.Run()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Close the write end of the pipe and restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read the captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := strings.TrimSpace(buf.String())

			// Check if the output contains the expected message
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Output message incorrect.\nExpected: %s\nGot: %s",
					tt.expectedOutput, output)
			}
		})
	}
}
