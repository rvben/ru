package update

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

// MockPyPI implements the PackageManager interface for testing
type MockPyPI struct {
	versions map[string]string
}

func (m *MockPyPI) GetLatestVersion(packageName string) (string, error) {
	if version, ok := m.versions[packageName]; ok {
		return version, nil
	}
	return "", fmt.Errorf("package not found")
}

func (m *MockPyPI) SetCustomIndexURL() error {
	return nil
}

func TestUpdateRequirementsFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Change to the temp directory for the test
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer os.Chdir(currentDir)

	// Create a test requirements file
	testFile := "requirements.txt"
	content := `package1==1.0.0
package2>=2.0.0,<3.0.0
package3
`
	err = ioutil.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create an updater with test settings
	updater := New(true, false, []string{"."})
	// Replace the PyPI client with our mock
	updater.pypi = &MockPyPI{
		versions: map[string]string{
			"package1": "1.1.0",
			"package2": "2.5.0",
			"package3": "3.0.0",
		},
	}

	// Run the update
	err = updater.ProcessDirectory(".")
	if err != nil {
		t.Fatalf("ProcessDirectory failed: %v", err)
	}

	// Read the updated file
	updatedContent, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	// Check the updated content
	expectedContent := `package1==1.1.0
package2>=2.0.0,<3.0.0
package3==3.0.0
`
	if string(updatedContent) != expectedContent {
		t.Errorf("Updated content does not match expected.\nExpected:\n%s\nGot:\n%s", expectedContent, string(updatedContent))
	}

	// Check the update statistics
	if updater.filesUpdated != 1 {
		t.Errorf("Expected 1 file updated, got %d", updater.filesUpdated)
	}
	if updater.modulesUpdated != 2 {
		t.Errorf("Expected 2 modules updated, got %d", updater.modulesUpdated)
	}
}

func TestCheckVersionConstraints(t *testing.T) {
	updater := New(true, false, nil)

	tests := []struct {
		name       string
		currentVer string
		latestVer  string
		wantUpdate bool
		wantErr    bool
	}{
		{
			name:       "Simple version update",
			currentVer: "==1.0.0",
			latestVer:  "1.1.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "Version with range",
			currentVer: ">=1.0.0,<2.0.0",
			latestVer:  "1.5.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "Invalid version",
			currentVer: "invalid",
			latestVer:  "1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldUpdate, err := updater.checkVersionConstraints(tt.latestVer, tt.currentVer)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkVersionConstraints() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if shouldUpdate != tt.wantUpdate {
				t.Errorf("checkVersionConstraints() = %v, want %v", shouldUpdate, tt.wantUpdate)
			}
		})
	}
}
