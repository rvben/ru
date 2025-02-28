package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOutputFormat(t *testing.T) {
	// Skip this test if running in continuous integration or if the GO_WANT_HELPER_PROCESS is set
	if os.Getenv("CI") != "" || os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
		t.Skip("Skipping test in CI environment")
	}

	// Get the current directory - we are in cmd/ru
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Build the command for testing
	testBinaryPath := filepath.Join(cwd, "ru_test_binary")
	buildCmd := exec.Command("go", "build", "-o", testBinaryPath)
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build test binary: %v\n%s", err, buildOutput)
	}
	defer os.Remove(testBinaryPath)

	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "ru-cli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name                string
		requirementsContent string
		expectedOutput      string
	}{
		{
			name:                "Single package update",
			requirementsContent: "requests==2.28.0\n",
			expectedOutput:      "1 file updated with 1 package upgraded",
		},
		{
			name:                "Multiple packages update",
			requirementsContent: "requests==2.28.0\nflask==2.0.0\n",
			expectedOutput:      "1 file updated with 2 packages upgraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test directory for this test case
			testCaseDir := filepath.Join(tempDir, strings.ReplaceAll(tt.name, " ", "-"))
			err := os.MkdirAll(testCaseDir, 0755)
			if err != nil {
				t.Fatalf("Failed to create test case directory: %v", err)
			}

			// Create requirements.txt with older versions
			requirementsPath := filepath.Join(testCaseDir, "requirements.txt")
			err = os.WriteFile(requirementsPath, []byte(tt.requirementsContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write requirements file: %v", err)
			}

			// Create mock cache for testing
			mockCacheDir := filepath.Join(testCaseDir, ".ru", "cache")
			err = os.MkdirAll(mockCacheDir, 0755)
			if err != nil {
				t.Fatalf("Failed to create mock cache directory: %v", err)
			}

			// Create mock cache files for requests and flask
			mockCache := map[string]string{
				"requests": "2.32.3",
				"flask":    "3.1.0",
			}
			for pkg, version := range mockCache {
				cacheFile := filepath.Join(mockCacheDir, fmt.Sprintf("%s.json", pkg))
				cacheContent := fmt.Sprintf(`{"latest":"%s"}`, version)
				err = os.WriteFile(cacheFile, []byte(cacheContent), 0644)
				if err != nil {
					t.Fatalf("Failed to write mock cache file: %v", err)
				}
			}

			// Run the command with redirected output
			var outputBuffer bytes.Buffer
			cmd := exec.Command(testBinaryPath, "update")
			cmd.Dir = testCaseDir
			cmd.Stdout = &outputBuffer
			cmd.Stderr = os.Stderr
			cmd.Env = append(os.Environ(),
				"RU_TEST_MODE=1", // Set test mode to avoid actual network calls
				fmt.Sprintf("RU_CACHE_DIR=%s", mockCacheDir))

			if err := cmd.Run(); err != nil {
				t.Fatalf("Command failed: %v\nOutput: %s", err, outputBuffer.String())
			}

			// Get the output
			output := outputBuffer.String()

			// Check if output contains expected message
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got:\n%s",
					tt.expectedOutput, output)
			}
		})
	}
}

// Helper function for tests that set up mock environment
func setupMockEnv(t *testing.T, packages map[string]string) (string, func()) {
	dir, err := os.MkdirTemp("", "ru-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Create mock cache directory
	cacheDir := filepath.Join(dir, ".ru", "cache")
	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	// Create mock cache files for each package
	for pkg, version := range packages {
		pkgFile := filepath.Join(cacheDir, fmt.Sprintf("%s.json", pkg))
		content := fmt.Sprintf(`{"latest": "%s"}`, version)
		err = os.WriteFile(pkgFile, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write cache file for %s: %v", pkg, err)
		}
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return dir, cleanup
}
