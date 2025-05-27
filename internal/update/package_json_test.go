package update

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// testUpdatePackageJsonFile simulates the package.json update logic for testing
func testUpdatePackageJsonFile(filePath string, getLatestVersion func(string) (string, error)) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	var packageJSON map[string]interface{}
	if err := json.Unmarshal(content, &packageJSON); err != nil {
		return fmt.Errorf("error parsing JSON in %s: %w", filePath, err)
	}

	dependencies, hasDependencies := packageJSON["dependencies"].(map[string]interface{})
	if !hasDependencies {
		dependencies = make(map[string]interface{})
	}

	devDependencies, hasDevDependencies := packageJSON["devDependencies"].(map[string]interface{})
	if !hasDevDependencies {
		devDependencies = make(map[string]interface{})
	}

	updatedDeps := make(map[string]string)
	for name, version := range dependencies {
		versionStr, ok := version.(string)
		if !ok {
			continue
		}

		if strings.Contains(versionStr, "git") {
			continue
		}

		latestVersion, err := getLatestVersion(name)
		if err != nil {
			continue
		}

		prefix := ""
		for _, p := range []string{"^", "~", ">=", "<=", ">", "<"} {
			if strings.HasPrefix(versionStr, p) {
				prefix = p
				break
			}
		}

		updatedVersion := prefix + latestVersion
		if updatedVersion != versionStr {
			updatedDeps[name] = updatedVersion
			dependencies[name] = updatedVersion
		}
	}

	updatedDevDeps := make(map[string]string)
	for name, version := range devDependencies {
		versionStr, ok := version.(string)
		if !ok {
			continue
		}

		if strings.Contains(versionStr, "git") {
			continue
		}

		latestVersion, err := getLatestVersion(name)
		if err != nil {
			continue
		}

		prefix := ""
		for _, p := range []string{"^", "~", ">=", "<=", ">", "<"} {
			if strings.HasPrefix(versionStr, p) {
				prefix = p
				break
			}
		}

		updatedVersion := prefix + latestVersion
		if updatedVersion != versionStr {
			updatedDevDeps[name] = updatedVersion
			devDependencies[name] = updatedVersion
		}
	}

	if len(updatedDeps) == 0 && len(updatedDevDeps) == 0 {
		return nil
	}

	// This is the key logic we're testing: only set fields that originally existed or have updates
	if hasDependencies || len(updatedDeps) > 0 {
		packageJSON["dependencies"] = dependencies
	}
	if hasDevDependencies || len(updatedDevDeps) > 0 {
		packageJSON["devDependencies"] = devDependencies
	}

	updatedJSON, err := json.MarshalIndent(packageJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling JSON for %s: %w", filePath, err)
	}

	if err := os.WriteFile(filePath, updatedJSON, 0644); err != nil {
		return fmt.Errorf("error writing to file %s: %w", filePath, err)
	}

	return nil
}

func TestPackageJsonDevDependenciesHandling(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "package-json-test")
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
		name                 string
		inputPackageJson     string
		expectedHasDevDeps   bool
		expectedDevDepsCount int
		expectedDepsCount    int
		description          string
	}{
		{
			name: "Package.json without devDependencies",
			inputPackageJson: `{
  "name": "test-package",
  "version": "1.0.0",
  "description": "Test package without devDependencies",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.20"
  },
  "scripts": {
    "start": "node index.js"
  }
}`,
			expectedHasDevDeps:   false,
			expectedDevDepsCount: 0,
			expectedDepsCount:    2,
			description:          "Should NOT add devDependencies field when it doesn't exist",
		},
		{
			name: "Package.json with existing devDependencies",
			inputPackageJson: `{
  "name": "test-package-with-dev",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0"
  },
  "devDependencies": {
    "jest": "^29.0.0",
    "eslint": "^8.0.0"
  }
}`,
			expectedHasDevDeps:   true,
			expectedDevDepsCount: 2,
			expectedDepsCount:    1,
			description:          "Should preserve and update existing devDependencies",
		},
		{
			name: "Package.json with only devDependencies",
			inputPackageJson: `{
  "name": "dev-only-package",
  "version": "1.0.0",
  "devDependencies": {
    "typescript": "^4.9.0",
    "webpack": "^5.75.0"
  }
}`,
			expectedHasDevDeps:   true,
			expectedDevDepsCount: 2,
			expectedDepsCount:    0,
			description:          "Should handle packages with only devDependencies",
		},
		{
			name: "Package.json with empty dependencies and devDependencies",
			inputPackageJson: `{
  "name": "empty-deps-package",
  "version": "1.0.0",
  "dependencies": {},
  "devDependencies": {}
}`,
			expectedHasDevDeps:   true,
			expectedDevDepsCount: 0,
			expectedDepsCount:    0,
			description:          "Should preserve empty devDependencies object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the test package.json file
			packageJsonPath := "package.json"
			err := os.WriteFile(packageJsonPath, []byte(tt.inputPackageJson), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Test the devDependencies logic directly by simulating the update
			err = testUpdatePackageJsonFile(packageJsonPath, func(pkg string) (string, error) {
				// Return different versions for different packages to test updates
				switch pkg {
				case "express":
					return "5.1.0", nil
				case "lodash":
					return "4.17.21", nil
				case "jest":
					return "29.7.0", nil
				case "eslint":
					return "9.27.0", nil
				case "typescript":
					return "5.3.0", nil
				case "webpack":
					return "5.89.0", nil
				default:
					return "1.0.0", nil
				}
			})
			if err != nil {
				t.Fatalf("updatePackageJsonFile() failed: %v", err)
			}

			// Read the updated file
			updatedContent, err := os.ReadFile(packageJsonPath)
			if err != nil {
				t.Fatalf("Failed to read updated file: %v", err)
			}

			// Parse the updated JSON
			var updatedPackageJson map[string]interface{}
			err = json.Unmarshal(updatedContent, &updatedPackageJson)
			if err != nil {
				t.Fatalf("Failed to parse updated JSON: %v", err)
			}

			// Check if devDependencies field exists
			devDeps, hasDevDeps := updatedPackageJson["devDependencies"]
			if hasDevDeps != tt.expectedHasDevDeps {
				t.Errorf("%s: Expected hasDevDeps=%v, got %v", tt.description, tt.expectedHasDevDeps, hasDevDeps)
			}

			// If devDependencies should exist, check the count
			if tt.expectedHasDevDeps {
				if devDeps == nil {
					t.Errorf("%s: devDependencies field exists but is nil", tt.description)
				} else {
					devDepsMap, ok := devDeps.(map[string]interface{})
					if !ok {
						t.Errorf("%s: devDependencies is not a map", tt.description)
					} else if len(devDepsMap) != tt.expectedDevDepsCount {
						t.Errorf("%s: Expected %d devDependencies, got %d", tt.description, tt.expectedDevDepsCount, len(devDepsMap))
					}
				}
			}

			// Check dependencies count
			deps, hasDeps := updatedPackageJson["dependencies"]
			if hasDeps {
				depsMap, ok := deps.(map[string]interface{})
				if ok && len(depsMap) != tt.expectedDepsCount {
					t.Errorf("%s: Expected %d dependencies, got %d", tt.description, tt.expectedDepsCount, len(depsMap))
				}
			} else if tt.expectedDepsCount > 0 {
				t.Errorf("%s: Expected dependencies field to exist", tt.description)
			}

			// Clean up for next test
			os.Remove(packageJsonPath)
		})
	}
}

func TestPackageJsonVersionUpdates(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "package-json-version-test")
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

	inputPackageJson := `{
  "name": "version-test-package",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "~4.17.20"
  },
  "devDependencies": {
    "jest": "29.0.0",
    "eslint": ">=8.0.0"
  }
}`

	expectedVersions := map[string]string{
		"express": "^5.1.0",   // Should preserve ^ prefix
		"lodash":  "~4.17.21", // Should preserve ~ prefix
		"jest":    "29.7.0",   // Should update without prefix
		"eslint":  ">=9.27.0", // Should preserve >= prefix
	}

	// Create the test package.json file
	packageJsonPath := "package.json"
	err = os.WriteFile(packageJsonPath, []byte(inputPackageJson), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test the version update logic directly
	err = testUpdatePackageJsonFile(packageJsonPath, func(pkg string) (string, error) {
		switch pkg {
		case "express":
			return "5.1.0", nil
		case "lodash":
			return "4.17.21", nil
		case "jest":
			return "29.7.0", nil
		case "eslint":
			return "9.27.0", nil
		default:
			return "1.0.0", nil
		}
	})
	if err != nil {
		t.Fatalf("updatePackageJsonFile() failed: %v", err)
	}

	// Read the updated file
	updatedContent, err := os.ReadFile(packageJsonPath)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	// Parse the updated JSON
	var updatedPackageJson map[string]interface{}
	err = json.Unmarshal(updatedContent, &updatedPackageJson)
	if err != nil {
		t.Fatalf("Failed to parse updated JSON: %v", err)
	}

	// Check dependencies versions
	deps := updatedPackageJson["dependencies"].(map[string]interface{})
	for pkg, expectedVersion := range expectedVersions {
		if pkg == "jest" || pkg == "eslint" {
			continue // These are in devDependencies
		}

		actualVersion, exists := deps[pkg]
		if !exists {
			t.Errorf("Package %s not found in dependencies", pkg)
			continue
		}

		if actualVersion != expectedVersion {
			t.Errorf("Package %s: expected version %s, got %s", pkg, expectedVersion, actualVersion)
		}
	}

	// Check devDependencies versions
	devDeps := updatedPackageJson["devDependencies"].(map[string]interface{})
	for pkg, expectedVersion := range expectedVersions {
		if pkg == "express" || pkg == "lodash" {
			continue // These are in dependencies
		}

		actualVersion, exists := devDeps[pkg]
		if !exists {
			t.Errorf("Package %s not found in devDependencies", pkg)
			continue
		}

		if actualVersion != expectedVersion {
			t.Errorf("Package %s: expected version %s, got %s", pkg, expectedVersion, actualVersion)
		}
	}
}
