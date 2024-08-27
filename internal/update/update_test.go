package update

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// MockPackageManager implements the PackageManager interface for testing
type MockPackageManager struct {
	versions map[string]string
}

func (m *MockPackageManager) GetLatestVersion(packageName string) (string, error) {
	return m.versions[packageName], nil
}

func (m *MockPackageManager) SetCustomIndexURL() error {
	return nil
}

func TestUpdateRequirementsFile(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test requirements file
	testFile := filepath.Join(tempDir, "requirements.txt")
	content := `
package1==1.0.0
package2>=2.0.0,<3.0.0
package3
`
	err = ioutil.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a mock package manager
	mockPM := &MockPackageManager{
		versions: map[string]string{
			"package1": "1.1.0",
			"package2": "2.5.0",
			"package3": "3.0.0",
		},
	}

	// Create an updater with the mock package manager
	updater := NewUpdater(mockPM)

	// Run the update
	err = updater.ProcessDirectory(tempDir)
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
	updater := NewUpdater(nil)

	testCases := []struct {
		latestVersion      string
		versionConstraints string
		expected           bool
	}{
		{"2.0.0", ">=1.0.0,<3.0.0", true},
		{"4.0.0", ">=1.0.0,<3.0.0", false},
		{"1.5.0", "~=1.0", true},
		{"2.0.0", "~=1.0", false},
		{"3.0.0", "~=1.0", false},
		{"1.1.0", "~=1.1", true},
		{"1.2.0", "~=1.1", true},
		{"2.0.0", "~=1.1", false},
		{"1.1.1", "~=1.1.0", true},
		{"1.1.2", "~=1.1.0", true},
		{"1.2.0", "~=1.1.0", false},
		{"1.1.0", "==1.1.0", true},
		{"1.1.1", "==1.1.0", false},
	}

	for _, tc := range testCases {
		result, err := updater.checkVersionConstraints(tc.latestVersion, tc.versionConstraints)
		if err != nil {
			t.Errorf("checkVersionConstraints(%s, %s) returned error: %v", tc.latestVersion, tc.versionConstraints, err)
			continue
		}
		if result != tc.expected {
			t.Errorf("checkVersionConstraints(%s, %s) = %v, expected %v", tc.latestVersion, tc.versionConstraints, result, tc.expected)
		}
	}
}
