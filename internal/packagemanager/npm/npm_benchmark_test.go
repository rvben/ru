package npm

import (
	"encoding/json"
	"os"
	"testing"
)

// MockNPM is a mock implementation for benchmarking
type MockNPM struct {
	filePath string
}

// NewMockNPM creates a new mock NPM instance
func NewMockNPM(filePath string) *MockNPM {
	return &MockNPM{
		filePath: filePath,
	}
}

// LoadAndUpdate loads and updates a package.json file (mock implementation for benchmarks)
func (n *MockNPM) LoadAndUpdate(versions map[string]string) (bool, error) {
	// Read the file
	content, err := os.ReadFile(n.filePath)
	if err != nil {
		return false, err
	}

	// Parse JSON
	var packageJSON map[string]interface{}
	if err := json.Unmarshal(content, &packageJSON); err != nil {
		return false, err
	}

	// Process dependencies
	updated := false
	if deps, ok := packageJSON["dependencies"].(map[string]interface{}); ok {
		if processDependencySection(deps, versions) {
			updated = true
		}
	}

	if devDeps, ok := packageJSON["devDependencies"].(map[string]interface{}); ok {
		if processDependencySection(devDeps, versions) {
			updated = true
		}
	}

	// If updated, write back to file
	if updated {
		newContent, err := json.MarshalIndent(packageJSON, "", "  ")
		if err != nil {
			return false, err
		}
		if err := os.WriteFile(n.filePath, newContent, 0644); err != nil {
			return false, err
		}
	}

	return updated, nil
}

// processDependencySection processes a dependency section
func processDependencySection(deps map[string]interface{}, versions map[string]string) bool {
	updated := false
	for pkg, ver := range deps {
		currentVer, ok := ver.(string)
		if !ok {
			continue
		}
		if newVer, ok := updateVersion(currentVer, pkg, versions); ok {
			deps[pkg] = newVer
			updated = true
		}
	}
	return updated
}

// updateVersion updates a version string using npm's semver rules
func updateVersion(currentVer, pkg string, versions map[string]string) (string, bool) {
	latestVer, ok := versions[pkg]
	if !ok {
		return currentVer, false
	}

	// Determine prefix based on current version
	var prefix string
	if len(currentVer) > 0 {
		switch currentVer[0] {
		case '^':
			prefix = "^"
			currentVer = currentVer[1:]
		case '~':
			prefix = "~"
			currentVer = currentVer[1:]
		case '>':
			if len(currentVer) > 1 && currentVer[1] == '=' {
				prefix = ">="
				currentVer = currentVer[2:]
			} else {
				prefix = ">"
				currentVer = currentVer[1:]
			}
		}
	}

	// For simplicity in benchmarks, just return the latest version with the same prefix
	return prefix + latestVer, true
}

// BenchmarkLoadAndUpdate benchmarks loading and updating a package.json file
func BenchmarkLoadAndUpdate(b *testing.B) {
	// Create a test package.json file with various dependencies
	content := `{
  "name": "benchmark-project",
  "version": "1.0.0",
  "description": "A project for benchmarking ru",
  "main": "index.js",
  "dependencies": {
    "express": "^4.17.1",
    "react": "17.0.2",
    "react-dom": "17.0.2",
    "lodash": "~4.17.20",
    "axios": "0.21.1",
    "moment": "2.29.1",
    "uuid": "^8.3.2",
    "chalk": "4.1.0",
    "commander": "^7.1.0",
    "dotenv": "8.2.0"
  },
  "devDependencies": {
    "jest": "26.6.3",
    "eslint": "^7.22.0",
    "prettier": "2.2.1",
    "typescript": "^4.2.3",
    "webpack": "5.26.0",
    "babel-loader": "^8.2.2"
  },
  "scripts": {
    "start": "node index.js",
    "test": "jest",
    "lint": "eslint ."
  },
  "keywords": [
    "benchmark",
    "test"
  ],
  "author": "ru",
  "license": "MIT"
}`

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "package-benchmark*.json")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write the test content
	if err := os.WriteFile(tmpfile.Name(), []byte(content), 0644); err != nil {
		b.Fatal(err)
	}

	// Prepare version map
	versions := map[string]string{
		"express":      "4.18.2",
		"react":        "18.2.0",
		"react-dom":    "18.2.0",
		"lodash":       "4.17.21",
		"axios":        "1.3.4",
		"moment":       "2.29.4",
		"uuid":         "9.0.0",
		"chalk":        "5.2.0",
		"commander":    "10.0.0",
		"dotenv":       "16.0.3",
		"jest":         "29.5.0",
		"eslint":       "8.36.0",
		"prettier":     "2.8.4",
		"typescript":   "5.0.2",
		"webpack":      "5.76.2",
		"babel-loader": "9.1.2",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new MockNPM instance for each iteration
		n := NewMockNPM(tmpfile.Name())
		_, err := n.LoadAndUpdate(versions)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUpdateVersion benchmarks updating a version string with npm's specific semver rules
func BenchmarkUpdateVersion(b *testing.B) {
	// Test cases for various version patterns
	testCases := []struct {
		name     string
		current  string
		pkg      string
		versions map[string]string
	}{
		{
			name:     "Caret range",
			current:  "^4.17.1",
			pkg:      "express",
			versions: map[string]string{"express": "4.18.2"},
		},
		{
			name:     "Tilde range",
			current:  "~4.17.20",
			pkg:      "lodash",
			versions: map[string]string{"lodash": "4.17.21"},
		},
		{
			name:     "Exact version",
			current:  "17.0.2",
			pkg:      "react",
			versions: map[string]string{"react": "18.2.0"},
		},
		{
			name:     "Star range",
			current:  "4.*",
			pkg:      "commander",
			versions: map[string]string{"commander": "10.0.0"},
		},
		{
			name:     "X-range",
			current:  "4.x",
			pkg:      "dotenv",
			versions: map[string]string{"dotenv": "16.0.3"},
		},
		{
			name:     "Greater than",
			current:  ">7.0.0",
			pkg:      "eslint",
			versions: map[string]string{"eslint": "8.36.0"},
		},
		{
			name:     "Complex range",
			current:  ">=4.0.0 <5.0.0",
			pkg:      "webpack",
			versions: map[string]string{"webpack": "5.76.2"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tc := testCases[i%len(testCases)]
		updateVersion(tc.current, tc.pkg, tc.versions)
	}
}

// BenchmarkProcessDependencySection benchmarks processing a dependency section
func BenchmarkProcessDependencySection(b *testing.B) {
	// Create test data with various dependency patterns
	dependencies := map[string]interface{}{
		"express":      "^4.17.1",
		"react":        "17.0.2",
		"react-dom":    "17.0.2",
		"lodash":       "~4.17.20",
		"axios":        "0.21.1",
		"moment":       "2.29.1",
		"uuid":         "^8.3.2",
		"chalk":        "4.1.0",
		"commander":    "^7.1.0",
		"dotenv":       "8.2.0",
		"jest":         "26.6.3",
		"eslint":       "^7.22.0",
		"prettier":     "2.2.1",
		"typescript":   "^4.2.3",
		"webpack":      "5.26.0",
		"babel-loader": "^8.2.2",
	}

	// Prepare version map
	versions := map[string]string{
		"express":      "4.18.2",
		"react":        "18.2.0",
		"react-dom":    "18.2.0",
		"lodash":       "4.17.21",
		"axios":        "1.3.4",
		"moment":       "2.29.4",
		"uuid":         "9.0.0",
		"chalk":        "5.2.0",
		"commander":    "10.0.0",
		"dotenv":       "16.0.3",
		"jest":         "29.5.0",
		"eslint":       "8.36.0",
		"prettier":     "2.8.4",
		"typescript":   "5.0.2",
		"webpack":      "5.76.2",
		"babel-loader": "9.1.2",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Make a copy of the dependencies map to avoid modifying the original
		depsCopy := make(map[string]interface{}, len(dependencies))
		for k, v := range dependencies {
			depsCopy[k] = v
		}
		processDependencySection(depsCopy, versions)
	}
}
