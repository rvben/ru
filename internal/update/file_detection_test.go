package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectFileType(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "file-detection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name         string
		filename     string
		content      string
		expectedType string
		wantErr      bool
	}{
		{
			name:         "Standard requirements.txt",
			filename:     "requirements.txt",
			content:      "requests==2.25.0\nflask==1.1.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "Non-standard requirements file",
			filename:     "my-requirements.txt",
			content:      "django==3.2.0\npytest>=6.0.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "Dev requirements file",
			filename:     "requirements-dev.txt",
			content:      "black==21.0.0\nflake8>=3.8.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "Test requirements file",
			filename:     "test-requirements.txt",
			content:      "pytest==6.2.0\ncoverage>=5.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "Standard package.json",
			filename:     "package.json",
			content:      `{"name": "test", "version": "1.0.0", "dependencies": {"express": "^4.18.0"}}`,
			expectedType: "package.json",
			wantErr:      false,
		},
		{
			name:         "Non-standard package.json",
			filename:     "my-package.json",
			content:      `{"name": "my-app", "version": "2.0.0", "devDependencies": {"jest": "^29.0.0"}}`,
			expectedType: "package.json",
			wantErr:      false,
		},
		{
			name:         "Standard pyproject.toml",
			filename:     "pyproject.toml",
			content:      `[project]\nname = "test"\ndependencies = ["requests==2.25.0"]\n`,
			expectedType: "pyproject.toml",
			wantErr:      false,
		},
		{
			name:         "Non-standard pyproject.toml",
			filename:     "custom-pyproject.toml",
			content:      `[tool.poetry]\nname = "test"\n[tool.poetry.dependencies]\npython = "^3.8"\n`,
			expectedType: "pyproject.toml",
			wantErr:      false,
		},
		{
			name:         "Text file with requirements content",
			filename:     "deps.txt",
			content:      "numpy==1.21.0\nscipy>=1.7.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "JSON file that's not package.json",
			filename:     "config.json",
			content:      `{"database": "postgres", "port": 5432}`,
			expectedType: "unknown",
			wantErr:      true,
		},
		{
			name:         "TOML file that's not pyproject.toml",
			filename:     "config.toml",
			content:      `[server]\nhost = "localhost"\nport = 8080\n`,
			expectedType: "unknown",
			wantErr:      true,
		},
		{
			name:         "Empty text file",
			filename:     "empty.txt",
			content:      "",
			expectedType: "unknown",
			wantErr:      true,
		},
		{
			name:         "Requirements with index URL",
			filename:     "custom-deps.txt",
			content:      "-i https://pypi.org/simple/\nrequests==2.25.0\n",
			expectedType: "requirements",
			wantErr:      false,
		},
		{
			name:         "Requirements with comments only",
			filename:     "commented.txt",
			content:      "# This is a comment\n# Another comment\n",
			expectedType: "unknown",
			wantErr:      true,
		},
	}

	updater := New(true, false, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the test file
			filePath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Test file type detection
			detectedType, err := updater.detectFileType(filePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("detectFileType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && detectedType != tt.expectedType {
				t.Errorf("detectFileType() = %v, want %v", detectedType, tt.expectedType)
			}
		})
	}
}

func TestDirectFileProcessing(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "direct-file-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(currentDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	tests := []struct {
		name     string
		filename string
		content  string
		fileType string
	}{
		{
			name:     "Direct requirements file",
			filename: "my-deps.txt",
			content:  "requests==2.25.0\nflask==1.1.0\n",
			fileType: "requirements",
		},
		{
			name:     "Direct package.json file",
			filename: "app-package.json",
			content:  `{"name": "test", "version": "1.0.0", "dependencies": {"express": "^4.18.0"}}`,
			fileType: "package.json",
		},
		{
			name:     "Direct pyproject.toml file",
			filename: "project-config.toml",
			content: `[project]
name = "test"
dependencies = ["requests==2.25.0"]
`,
			fileType: "pyproject.toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the test file
			err := os.WriteFile(tt.filename, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create updater with mock package manager
			updater := New(true, false, []string{tt.filename})
			updater.pypi = &MockPackageManager{
				getLatestVersionFunc: func(pkg string) (string, error) {
					return "9.9.9", nil
				},
			}

			// Test direct file processing
			err = updater.Run()
			if err != nil {
				t.Errorf("Run() failed for direct file processing: %v", err)
			}

			// Verify the file was processed
			if updater.filesUpdated == 0 && updater.filesUnchanged == 0 {
				t.Errorf("No files were processed")
			}

			// Read the file to verify it was updated (for requirements and pyproject files)
			if tt.fileType == "requirements" || tt.fileType == "pyproject.toml" {
				content, err := os.ReadFile(tt.filename)
				if err != nil {
					t.Fatalf("Failed to read updated file: %v", err)
				}

				if tt.fileType == "requirements" && !strings.Contains(string(content), "9.9.9") {
					t.Errorf("Requirements file was not updated with latest versions")
				}
			}
		})
	}
}
