package pypi

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// simpleExtractVersions is a simplified version for testing that just looks for version keys
func simpleExtractVersions(data string) ([]string, error) {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
		return nil, err
	}

	releases, ok := jsonData["releases"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no releases found or invalid format")
	}

	var versions []string
	for version := range releases {
		versions = append(versions, version)
	}

	return versions, nil
}

// improvedExtractVersions uses token-based parsing for efficiency
func improvedExtractVersions(r io.Reader) ([]string, error) {
	dec := json.NewDecoder(r)

	// Find the releases object
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("releases not found")
		}
		if err != nil {
			return nil, err
		}

		// Found the releases field
		if t == "releases" {
			break
		}
	}

	// Skip the opening { of releases object
	_, err := dec.Token()
	if err != nil {
		return nil, err
	}

	var versions []string

	// Process all keys in the releases object
	for {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}

		// End of releases object
		if t == json.Delim('}') {
			break
		}

		// Add version key to list
		if version, ok := t.(string); ok {
			versions = append(versions, version)
		}

		// Skip the array associated with this version
		if err := skipArrayValue(dec); err != nil {
			return nil, err
		}
	}

	return versions, nil
}

// skipArrayValue skips the next value in the JSON decoder stream
func skipArrayValue(dec *json.Decoder) error {
	// Get the first token, which should be the opening delimiter
	t, err := dec.Token()
	if err != nil {
		return err
	}

	// If it's not a delimiter, nothing to skip
	if _, ok := t.(json.Delim); !ok {
		return nil
	}

	// Keep track of nesting
	depth := 1

	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return err
		}

		switch tk := t.(type) {
		case json.Delim:
			switch tk {
			case '[', '{':
				depth++
			case ']', '}':
				depth--
			}
		}
	}

	return nil
}

// Test function to verify our extraction works correctly
func TestExtractVersions(t *testing.T) {
	// Simple test JSON with a few versions
	testJSON := `{
		"info": { "name": "test" },
		"releases": {
			"1.0.0": [{}],
			"1.1.0": [{}],
			"2.0.0": [{}]
		}
	}`

	// Test the simple extraction
	simpleVersions, err := simpleExtractVersions(testJSON)
	if err != nil {
		t.Fatalf("Simple extraction failed: %v", err)
	}
	if len(simpleVersions) != 3 {
		t.Errorf("Simple: Expected 3 versions, got %d", len(simpleVersions))
	}

	// Test the improved extraction
	reader := strings.NewReader(testJSON)
	improvedVersions, err := improvedExtractVersions(reader)
	if err != nil {
		t.Fatalf("Improved extraction failed: %v", err)
	}
	if len(improvedVersions) != 3 {
		t.Errorf("Improved: Expected 3 versions, got %d", len(improvedVersions))
	}
}

func BenchmarkStandardJSON(b *testing.B) {
	// Create a sample PyPI JSON response
	jsonResponse := `{
		"info": {
			"name": "sample-package",
			"version": "1.0.0"
		},
		"releases": {
			"0.1.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-01-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/0.1.0/sample-0.1.0.tar.gz"
				}
			],
			"0.2.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-02-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/0.2.0/sample-0.2.0.tar.gz"
				}
			],
			"1.0.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-03-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/1.0.0/sample-1.0.0.tar.gz"
				}
			],
			"1.1.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-04-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/1.1.0/sample-1.1.0.tar.gz"
				}
			],
			"2.0.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-05-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/2.0.0/sample-2.0.0.tar.gz"
				}
			]
		}
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		versions, err := simpleExtractVersions(jsonResponse)
		if err != nil {
			b.Fatal(err)
		}

		if len(versions) != 5 {
			b.Fatalf("Expected 5 versions, got %d", len(versions))
		}
	}
}

func BenchmarkOptimizedJSON(b *testing.B) {
	// Create a sample PyPI JSON response
	jsonResponse := `{
		"info": {
			"name": "sample-package",
			"version": "1.0.0"
		},
		"releases": {
			"0.1.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-01-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/0.1.0/sample-0.1.0.tar.gz"
				}
			],
			"0.2.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-02-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/0.2.0/sample-0.2.0.tar.gz"
				}
			],
			"1.0.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-03-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/1.0.0/sample-1.0.0.tar.gz"
				}
			],
			"1.1.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-04-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/1.1.0/sample-1.1.0.tar.gz"
				}
			],
			"2.0.0": [
				{
					"packagetype": "sdist",
					"upload_time": "2020-05-01T00:00:00",
					"url": "https://files.pythonhosted.org/packages/sample/2.0.0/sample-2.0.0.tar.gz"
				}
			]
		}
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader := strings.NewReader(jsonResponse)
		versions, err := improvedExtractVersions(reader)
		if err != nil {
			b.Fatal(err)
		}

		if len(versions) != 5 {
			b.Fatalf("Expected 5 versions, got %d", len(versions))
		}
	}
}

func BenchmarkLargeJSONComparison(b *testing.B) {
	// Create a larger JSON response with many more releases
	var jsonBuilder strings.Builder
	jsonBuilder.WriteString(`{"info":{"name":"large-package","version":"1.0.0"},"releases":{`)

	// Add 100 releases with multiple builds per release
	for i := 0; i < 100; i++ {
		version := fmt.Sprintf("%d.%d.%d", i/10, i%10, i%5)
		if i > 0 {
			jsonBuilder.WriteString(",")
		}
		jsonBuilder.WriteString(fmt.Sprintf(`"%s":[`, version))

		// Add multiple build types for each release
		for j := 0; j < 3; j++ {
			if j > 0 {
				jsonBuilder.WriteString(",")
			}
			jsonBuilder.WriteString(fmt.Sprintf(`{
				"packagetype": "%s",
				"upload_time": "2020-%02d-%02dT00:00:00",
				"url": "https://files.pythonhosted.org/packages/large/%s/large-%s-%s.tar.gz"
			}`, []string{"sdist", "bdist_wheel", "bdist_egg"}[j%3], (i%12)+1, (j%28)+1, version, version, []string{"tar.gz", "whl", "egg"}[j%3]))
		}
		jsonBuilder.WriteString("]")
	}
	jsonBuilder.WriteString("}}") // Close releases and main object

	largeJSON := jsonBuilder.String()

	b.Run("Standard", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			versions, err := simpleExtractVersions(largeJSON)
			if err != nil {
				b.Fatal(err)
			}

			if len(versions) != 100 {
				b.Fatalf("Expected 100 versions, got %d", len(versions))
			}
		}
	})

	b.Run("Optimized", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			reader := strings.NewReader(largeJSON)
			versions, err := improvedExtractVersions(reader)
			if err != nil {
				b.Fatal(err)
			}

			if len(versions) != 100 {
				b.Fatalf("Expected 100 versions, got %d", len(versions))
			}
		}
	})
}
