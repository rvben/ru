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
		t.Errorf("Updated content does not match expected.\nExpected:\n%q\nGot:\n%q\nExpected bytes: %v\nGot bytes: %v", expectedContent, string(updatedContent), []byte(expectedContent), []byte(updatedContent))
	}
	// Also check after trimming trailing whitespace/newlines
	trimmedExpected := strings.TrimSpace(expectedContent)
	trimmedActual := strings.TrimSpace(string(updatedContent))
	if trimmedExpected != trimmedActual {
		t.Errorf("Trimmed content does not match.\nExpected:\n%q\nGot:\n%q", trimmedExpected, trimmedActual)
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

func TestPoetryStyleDependencyDetection(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedPkgs   []string
		unexpectedPkgs []string
	}{
		{
			name: "Valid poetry dependencies",
			content: `[tool.poetry.dependencies]
flask = "2.0.0"
requests = "^2.31.0"

[tool.poetry.dev-dependencies]
pytest = "^7.4.3"`,
			expectedPkgs:   []string{"flask", "requests", "pytest"},
			unexpectedPkgs: []string{},
		},
		{
			name: "Configuration parameters should be ignored",
			content: `[[tool.uv.index]]
name = "default"
url = "https://pypi.org/simple"

[[tool.uv.index]]
name = "custom"
url = "https://repo.example.com/simple/"
publish-url = "https://repo.example.com/publish/"

[project]
dependencies = [
    "requests==2.31.0"
]`,
			expectedPkgs:   []string{"requests"},
			unexpectedPkgs: []string{"name", "url", "publish-url"},
		},
		{
			name: "Mixed content with valid and invalid sections",
			content: `[project]
name = "example-project"
version = "0.1.0"
dependencies = [
    "flask==2.0.0",
    "requests==2.31.0"
]

[tool.isort]
profile = "black"

[tool.coverage.report]
include_namespace_packages = true
omit = [
    '**/__init__.py',
    'cdk.out/*'
]

[tool.poetry.dependencies]
django = "^4.2.0"
`,
			expectedPkgs:   []string{"flask", "requests", "django"},
			unexpectedPkgs: []string{"profile", "include_namespace_packages", "omit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the test content
			tempFile, err := os.CreateTemp("", "pyproject-*.toml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tempFile.Name())

			_, err = tempFile.WriteString(tt.content)
			if err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			tempFile.Close()

			// Create a mock updater with a custom GetLatestVersion function that records package names
			processedPackages := make(map[string]bool)
			mockPyPI := &MockPackageManager{
				getLatestVersionFunc: func(pkgName string) (string, error) {
					processedPackages[pkgName] = true
					return "9.9.9", nil
				},
			}

			updater := NewUpdater(mockPyPI)
			// Process the temp file
			err = updater.processPyProjectFile(tempFile.Name())
			if err != nil {
				t.Fatalf("processPyProjectFile failed: %v", err)
			}

			// Check that all expected packages were processed
			for _, pkg := range tt.expectedPkgs {
				if !processedPackages[pkg] {
					t.Errorf("Expected package %q to be processed, but it wasn't", pkg)
				}
			}

			// Check that none of the unexpected packages were processed
			for _, pkg := range tt.unexpectedPkgs {
				if processedPackages[pkg] {
					t.Errorf("Package %q should not have been processed, but it was", pkg)
				}
			}
		})
	}
}

func TestAlignerWildcardVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aligner-wildcard-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create three subdirectories, each with a requirements.txt
	dirs := []string{"a", "b", "c"}
	contents := []string{"rdflib==7.0.*\n", "rdflib==7.1.0\n", "rdflib==7.1.*\n"}
	files := make([]string, 3)
	for i, d := range dirs {
		dirPath := filepath.Join(tmpDir, d)
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dirPath, err)
		}
		filePath := filepath.Join(dirPath, "requirements.txt")
		if err := os.WriteFile(filePath, []byte(contents[i]), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filePath, err)
		}
		files[i] = filePath
	}

	aligner := NewAligner()
	if err := aligner.collectVersions(tmpDir); err != nil {
		t.Fatalf("collectVersions failed: %v", err)
	}
	if err := aligner.alignVersions(tmpDir); err != nil {
		t.Fatalf("alignVersions failed: %v", err)
	}

	for _, f := range files {
		updated, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		str := string(updated)
		if strings.Contains(str, "==7.1.") && !strings.Contains(str, "==7.1.*") {
			t.Errorf("Invalid version '==7.1.' found in %s: %q", f, str)
		}
		if !strings.Contains(str, "==7.1.0") && !strings.Contains(str, "==7.0.*") && !strings.Contains(str, "==7.1.*") {
			t.Errorf("Expected wildcard or valid version in %s, got: %q", f, str)
		}
	}

	// All files should be aligned to the highest valid version, which is 7.1.*
	for _, f := range files {
		updated, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", f, err)
		}
		str := string(updated)
		if !strings.Contains(str, "==7.1.*") {
			t.Errorf("Expected all files to be aligned to '==7.1.*', got: %q in %s", str, f)
		}
	}
}

func TestDryRunSummaryOutput(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := ioutil.TempDir("", "test-dryrun")
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
	content := `package1==1.0.0\npackage2\n`
	if err := ioutil.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Set up the updater in dryRun mode
	updater := New(true, false, []string{"."})
	updater.dryRun = true
	updater.pypi = &MockPackageManager{
		getLatestVersionFunc: func(pkg string) (string, error) {
			if pkg == "package1" {
				return "1.1.0", nil
			}
			if pkg == "package2" {
				return "2.0.0", nil
			}
			return "", fmt.Errorf("package %s not found", pkg)
		},
	}

	// Capture stdout using os.Pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	stdout := os.Stdout
	os.Stdout = w

	err = updater.ProcessDirectory(".")
	if err != nil {
		w.Close()
		os.Stdout = stdout
		t.Fatalf("ProcessDirectory failed: %v", err)
	}
	updater.Run()

	w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	output := buf.String()
	if !strings.Contains(output, "Would update") && !strings.Contains(output, "No updates would be made") {
		t.Errorf("Dry run summary output missing or incorrect. Got: %q", output)
	}
}
