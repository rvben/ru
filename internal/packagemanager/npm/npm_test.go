package npm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetLatestVersion(t *testing.T) {
	expectedVersion := "1.2.3"
	mockResponse := struct {
		Version string `json:"version"`
	}{Version: expectedVersion}
	responseData, _ := json.Marshal(mockResponse)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(responseData)
	}))
	defer ts.Close()

	npm := New()
	npm.SetCustomIndexURL(ts.URL)

	version, err := npm.GetLatestVersion("example-package")
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}

	if version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, version)
	}
}

func BenchmarkStandardNPMJSONProcessing(b *testing.B) {
	// Create a sample NPM JSON response
	jsonResponse := `{
		"name": "sample-package",
		"version": "2.1.0",
		"description": "A sample package for testing",
		"main": "index.js",
		"scripts": {
			"test": "echo \"Error: no test specified\" && exit 1"
		},
		"keywords": [
			"test",
			"sample"
		],
		"author": "Test Author",
		"license": "MIT",
		"dependencies": {
			"lodash": "^4.17.21",
			"express": "^4.17.1",
			"react": "^17.0.2"
		}
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new reader for each iteration
		reader := strings.NewReader(jsonResponse)

		// Use the old method
		var data struct {
			Version string `json:"version"`
		}
		err := json.NewDecoder(reader).Decode(&data)
		if err != nil {
			b.Fatal(err)
		}

		// Verify version
		if data.Version != "2.1.0" {
			b.Fatalf("Expected version 2.1.0, got %s", data.Version)
		}
	}
}

func BenchmarkOptimizedNPMJSONProcessing(b *testing.B) {
	// Create a sample NPM JSON response
	jsonResponse := `{
		"name": "sample-package",
		"version": "2.1.0",
		"description": "A sample package for testing",
		"main": "index.js",
		"scripts": {
			"test": "echo \"Error: no test specified\" && exit 1"
		},
		"keywords": [
			"test",
			"sample"
		],
		"author": "Test Author",
		"license": "MIT",
		"dependencies": {
			"lodash": "^4.17.21",
			"express": "^4.17.1",
			"react": "^17.0.2"
		}
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new reader for each iteration
		reader := strings.NewReader(jsonResponse)

		// Use the optimized method
		version, err := extractVersionFromNPMJSON(reader)
		if err != nil {
			b.Fatal(err)
		}

		// Verify version
		if version != "2.1.0" {
			b.Fatalf("Expected version 2.1.0, got %s", version)
		}
	}
}

func BenchmarkLargeNPMJSONProcessingComparison(b *testing.B) {
	// Create a larger JSON response
	var jsonBuilder strings.Builder
	jsonBuilder.WriteString(`{
		"name": "large-package",
		"version": "3.5.1",
		"description": "A large package for benchmark testing",
		"main": "index.js",
		"scripts": {
			"test": "jest",
			"start": "node index.js",
			"build": "webpack"
		},`)

	// Add a large number of keywords
	jsonBuilder.WriteString(`"keywords": [`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			jsonBuilder.WriteString(",")
		}
		jsonBuilder.WriteString(`"keyword-`)
		jsonBuilder.WriteString(strings.Repeat(string(rune('a'+i%26)), 20))
		jsonBuilder.WriteString(`"`)
	}
	jsonBuilder.WriteString(`],`)

	// Add a large dependencies object
	jsonBuilder.WriteString(`"dependencies": {`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			jsonBuilder.WriteString(",")
		}
		jsonBuilder.WriteString(`"package-`)
		jsonBuilder.WriteString(strings.Repeat(string(rune('a'+i%26)), 10))
		jsonBuilder.WriteString(`": "^`)
		jsonBuilder.WriteString(string(rune('0' + i/10)))
		jsonBuilder.WriteString(".")
		jsonBuilder.WriteString(string(rune('0' + i%10)))
		jsonBuilder.WriteString(".0\"")
	}
	jsonBuilder.WriteString(`}}`)

	largeJSON := jsonBuilder.String()

	b.Run("Standard", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			reader := strings.NewReader(largeJSON)

			var data struct {
				Version string `json:"version"`
			}
			err := json.NewDecoder(reader).Decode(&data)
			if err != nil {
				b.Fatal(err)
			}

			if data.Version != "3.5.1" {
				b.Fatalf("Expected version 3.5.1, got %s", data.Version)
			}
		}
	})

	b.Run("Optimized", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			reader := strings.NewReader(largeJSON)

			version, err := extractVersionFromNPMJSON(reader)
			if err != nil {
				b.Fatal(err)
			}

			if version != "3.5.1" {
				b.Fatalf("Expected version 3.5.1, got %s", version)
			}
		}
	})
}
