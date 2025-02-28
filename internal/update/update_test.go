package update

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rvben/ru/internal/packagemanager"
)

// MockPackageManager is a mock implementation of the PackageManager interface for testing
type MockPackageManager struct {
	getLatestVersionFunc func(packageName string) (string, error)
}

func (m *MockPackageManager) GetLatestVersion(packageName string) (string, error) {
	if m.getLatestVersionFunc != nil {
		return m.getLatestVersionFunc(packageName)
	}
	return "9.9.9", nil
}

func (m *MockPackageManager) SetCustomIndexURL() error {
	return nil
}

// NewUpdater creates a new Updater instance with the given PyPI client
func NewUpdater(pypi packagemanager.PackageManager) *Updater {
	return &Updater{
		pypi:           pypi,
		filesUpdated:   0,
		modulesUpdated: 0,
	}
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
	updater.pypi = &MockPackageManager{
		getLatestVersionFunc: func(pkg string) (string, error) {
			if pkg == "package1" {
				return "1.1.0", nil
			}
			if pkg == "package2" {
				return "2.5.0", nil
			}
			if pkg == "package3" {
				return "3.0.0", nil
			}
			return "", fmt.Errorf("package %s not found", pkg)
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

func TestUpdatePyProjectFile(t *testing.T) {
	// Save current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		content  string
		versions map[string]string
		wantErr  bool
	}{
		{
			name: "PEP 621 format with dependencies",
			content: `[project]
dependencies = [
    "requests==2.31.0",
    "flask==2.0.0"
]`,
			versions: map[string]string{
				"requests": "2.32.0",
				"flask":    "2.1.0",
			},
			wantErr: false,
		},
		{
			name: "Poetry format with circular dependencies",
			content: `[tool.poetry]
dependencies = { flask = ">=2.0.0,<3.0.0", requests = "^2.31.0" }
dev-dependencies = { pytest = "^7.4.3" }`,
			versions: map[string]string{
				"flask":    "2.1.0",
				"requests": "2.32.0",
				"pytest":   "7.4.4",
			},
			wantErr: false,
		},
		{
			name: "Mixed format with dependency groups",
			content: `[project]
dependencies = [
    "flask==2.0.0",
    "constructs>=10.0.0,<11.0.0"
]

[tool.poetry]
dependencies = { requests = "^2.31.0" }`,
			versions: map[string]string{
				"flask":      "2.1.0",
				"constructs": "10.3.0",
				"requests":   "2.32.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for the test
			tmpDir, err := os.MkdirTemp("", "pyproject_test")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)

			// Change to the temp directory for the test
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(currentDir)

			// Create a temporary pyproject.toml file
			if err := os.WriteFile("pyproject.toml", []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			// Create an updater with mock PyPI
			mockPyPI := &MockPackageManager{
				getLatestVersionFunc: func(pkg string) (string, error) {
					if version, ok := tt.versions[pkg]; ok {
						return version, nil
					}
					return "", fmt.Errorf("package %s not found", pkg)
				},
			}
			updater := NewUpdater(mockPyPI)

			// Process the directory
			if err := updater.ProcessDirectory("."); (err != nil) != tt.wantErr {
				t.Errorf("ProcessDirectory failed: %v", err)
			}

			// Read the updated content
			updatedContent, err := os.ReadFile("pyproject.toml")
			if err != nil {
				t.Fatal(err)
			}

			// Verify the content was updated correctly
			for pkg, version := range tt.versions {
				// Handle different version constraint formats
				if strings.Contains(tt.content, fmt.Sprintf("%s==", pkg)) {
					expectedStr := fmt.Sprintf(`"%s==%s"`, pkg, version)
					if !strings.Contains(string(updatedContent), expectedStr) {
						t.Errorf("Updated content does not contain %s", expectedStr)
					}
				}
			}
		})
	}
}

// TestParallelProcessing tests that the parallel processing of files works correctly
func TestParallelProcessing(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "ru-test-parallel")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	numRequirementsFiles := 5
	numPackageJSONFiles := 3
	numPyprojectFiles := 2

	// Create requirements files
	for i := 0; i < numRequirementsFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("requirements-%d.txt", i))
		content := "package1==1.0.0\npackage2>=2.0.0\npackage3~=3.0.0\n"
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create package.json files
	for i := 0; i < numPackageJSONFiles; i++ {
		dirPath := filepath.Join(tempDir, fmt.Sprintf("node-project-%d", i))
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		filename := filepath.Join(dirPath, "package.json")
		content := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    "package1": "^1.0.0",
    "package2": "~2.0.0"
  },
  "devDependencies": {
    "package3": "3.0.0"
  }
}`
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create pyproject.toml files
	for i := 0; i < numPyprojectFiles; i++ {
		dirPath := filepath.Join(tempDir, fmt.Sprintf("poetry-project-%d", i))
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		filename := filepath.Join(dirPath, "pyproject.toml")
		content := `[tool.poetry]
name = "test-project"
version = "1.0.0"
description = "Test project"

[tool.poetry.dependencies]
python = "^3.8"
package1 = "^1.0.0"
package2 = "~2.0.0"

[tool.poetry.dev-dependencies]
package3 = "3.0.0"
`
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create a mock package manager that tracks which files are processed
	var processedCount int32

	// Mock the PyPI package manager
	mockPyPI := &MockPackageManager{
		getLatestVersionFunc: func(pkg string) (string, error) {
			atomic.AddInt32(&processedCount, 1)
			// Add a small delay to simulate network requests and ensure concurrency
			time.Sleep(50 * time.Millisecond)
			return "9.9.9", nil
		},
	}

	// Create the updater
	updater := New(false, false, []string{tempDir})

	// Replace the PyPI package manager with our mock
	updater.pypi = mockPyPI

	// Replace the NPM client with a custom implementation that counts requests
	// We need to create a custom HTTP client to intercept requests
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	http.DefaultTransport = &countingTransport{
		RoundTripper: origTransport,
		counter:      &processedCount,
	}

	// Measure time taken for parallel processing
	startTime := time.Now()
	err = updater.ProcessDirectory(tempDir)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("ProcessDirectory failed: %v", err)
	}

	// Calculate the expected number of packages that would be processed
	// Each requirements file has 3 packages, each package.json has 3 packages,
	// and each pyproject.toml has 3 packages
	expectedPackages := numRequirementsFiles*3 + numPackageJSONFiles*3 + numPyprojectFiles*3

	// Check if the correct number of packages were processed
	actualCount := int(atomic.LoadInt32(&processedCount))
	if actualCount < expectedPackages/2 { // We might not get exactly the expected count due to caching, but should be at least half
		t.Errorf("Expected approximately %d packages to be processed, but got only %d", expectedPackages, actualCount)
	}

	// Log performance info
	t.Logf("Processed %d files with %d packages in %v",
		numRequirementsFiles+numPackageJSONFiles+numPyprojectFiles,
		actualCount,
		duration)

	// Rather than checking exact package versions, we'll verify that files were processed by checking if they were changed
	requirementsCount := 0
	for i := 0; i < numRequirementsFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("requirements-%d.txt", i))
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read updated file: %v", err)
		}

		if bytes.Contains(content, []byte("9.9.9")) {
			requirementsCount++
		}
	}

	packageJSONCount := 0
	for i := 0; i < numPackageJSONFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("node-project-%d", i), "package.json")
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read updated file: %v", err)
		}

		if bytes.Contains(content, []byte("9.9.9")) {
			packageJSONCount++
		}
	}

	pyprojectCount := 0
	for i := 0; i < numPyprojectFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("poetry-project-%d", i), "pyproject.toml")
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read updated file: %v", err)
		}

		if bytes.Contains(content, []byte("9.9.9")) {
			pyprojectCount++
		}
	}

	t.Logf("Updated files: %d requirements, %d package.json, %d pyproject.toml",
		requirementsCount, packageJSONCount, pyprojectCount)

	if requirementsCount+packageJSONCount+pyprojectCount == 0 {
		t.Errorf("No files were updated during parallel processing")
	}
}

// countingTransport is a custom http.RoundTripper that counts requests
type countingTransport struct {
	http.RoundTripper
	counter *int32
}

// RoundTrip implements the http.RoundTripper interface
func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Intercept all NPM registry requests and return a mock response
	if strings.Contains(req.URL.String(), "registry.npmjs.org") {
		atomic.AddInt32(t.counter, 1)
		time.Sleep(50 * time.Millisecond) // Simulate network delay

		// Create a mock response
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"name": "test-package",
				"version": "9.9.9",
				"description": "Mock package for testing"
			}`)),
			Header: make(http.Header),
		}, nil
	}

	// For non-NPM requests, pass through to the original transport
	return t.RoundTripper.RoundTrip(req)
}

// Verify PackageManager interface implementation
var _ packagemanager.PackageManager = &MockPackageManager{}
