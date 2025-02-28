package pypi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MockPyPIResponse structure for the JSON response from PyPI
type MockPyPIResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

func TestGetLatestVersion(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the request path contains the expected package name
		if strings.Contains(r.URL.Path, "/example-package/") {
			// Return a mock PyPI JSON response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{
				"info": {
					"name": "example-package",
					"version": "1.0.0"
				},
				"releases": {
					"1.0.0": [
						{
							"packagetype": "sdist",
							"upload_time": "2020-01-01T00:00:00",
							"url": "https://files.pythonhosted.org/packages/example/1.0.0/example-1.0.0.tar.gz"
						}
					],
					"1.1.0": [
						{
							"packagetype": "sdist",
							"upload_time": "2020-02-01T00:00:00",
							"url": "https://files.pythonhosted.org/packages/example/1.1.0/example-1.1.0.tar.gz"
						}
					],
					"2.0.0": [
						{
							"packagetype": "sdist",
							"upload_time": "2020-03-01T00:00:00",
							"url": "https://files.pythonhosted.org/packages/example/2.0.0/example-2.0.0.tar.gz"
						}
					]
				}
			}`)
			return
		}

		// Return 404 for other packages
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message": "Package not found"}`)
	}))
	defer server.Close()

	// Create a PyPI instance with our mock server URL
	pypi := New(true)
	pypi.pypiURL = server.URL

	// Test getting the latest version
	version, err := pypi.GetLatestVersion("example-package")
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}

	// Verify we got the expected version
	expectedVersion := "2.0.0"
	if version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, version)
	}
}

func TestGetLatestVersionError(t *testing.T) {
	// Create a mock HTTP server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message": "Package not found"}`)
	}))
	defer server.Close()

	// Create a PyPI instance with our mock server URL
	pypi := New(true)
	pypi.pypiURL = server.URL

	// Test getting the latest version for a non-existent package
	_, err := pypi.GetLatestVersion("non-existent-package")
	if err == nil {
		t.Error("Expected an error for non-existent package, got nil")
	}
}

func TestParseHTMLForLatestVersion(t *testing.T) {
	// Sample HTML input
	htmlContent := `<!DOCTYPE html>
	<html><head>
    <title>Links for tqdm</title>
	</head>
	<body>
		<h1>Links for tqdm</h1>
		<a href="4.66.5/tqdm-4.66.5-py3-none-any.whl#sha256=90279a3770753eafc9194a0364852159802111925aa30eb3f9d85b0e805ac7cd" data-requires-python=">=3.7" data-gpg-sig="false">tqdm-4.66.5-py3-none-any.whl</a>
		<br>
		<a href="4.66.5/tqdm-4.66.5.tar.gz#sha256=e1020aef2e5096702d8a025ac7d16b1577279c9d63f8375b63083e9a5f0fcbad" data-requires-python=">=3.7" data-gpg-sig="false">tqdm-4.66.5.tar.gz</a>
		<br>
		<a href="4.9.0/tqdm-4.9.0-py2.py3-none-any.whl#sha256=db1833247c074ee7189038d192d250e4bf650d11cec092bc9f686428d8b341c5" data-gpg-sig="false">tqdm-4.9.0-py2.py3-none-any.whl</a>
		<br>
		<a href="4.9.0/tqdm-4.9.0.tar.gz#sha256=acdfb7d746a76f742d38f4b473056b9e6fa92ddea12d7e0dafd1f537645e0c84" data-gpg-sig="false">tqdm-4.9.0.tar.gz</a>
		<br>
		<a href="4.9.0/tqdm-4.9.0.zip#sha256=e86a2166a99bd2b7ae2107cf9b6688b93dea74861fed81a35d0ab4619b168bb4" data-gpg-sig="false">tqdm-4.9.0.zip</a>
		<br>
	</body></html>`

	// Create a mock HTTP response from the HTML content
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(htmlContent)),
	}

	// Create a PyPI instance
	pypi := New(true)

	// Call the function to test
	latestVersion, err := pypi.parseHTMLForLatestVersion(response)
	if err != nil {
		t.Fatalf("parseHTMLForLatestVersion failed: %v", err)
	}

	// Define the expected latest version
	expectedVersion := "4.66.5"

	// Check if the returned latest version is correct
	if latestVersion != expectedVersion {
		t.Errorf("Expected latest version %s, but got %s", expectedVersion, latestVersion)
	}
}

func TestParseHTMLForLatestVersionPreferStable(t *testing.T) {
	// Sample HTML input with mixed stable and pre-release versions
	htmlContent := `<!DOCTYPE html>
	<html><head>
    <title>Links for pyyaml</title>
	</head>
	<body>
		<h1>Links for pyyaml</h1>
		<a href="6.0.1/pyyaml-6.0.1.tar.gz">pyyaml-6.0.1.tar.gz</a>
		<br>
		<a href="6.0.2rc1/pyyaml-6.0.2rc1.tar.gz">pyyaml-6.0.2rc1.tar.gz</a>
		<br>
		<a href="6.0.2/pyyaml-6.0.2.tar.gz">pyyaml-6.0.2.tar.gz</a>
		<br>
		<a href="6.0.3b1/pyyaml-6.0.3b1.tar.gz">pyyaml-6.0.3b1.tar.gz</a>
		<br>
	</body></html>`

	// Create a mock HTTP response from the HTML content
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(htmlContent)),
	}

	// Create a PyPI instance
	pypi := New(true)

	// Call the function to test
	latestVersion, err := pypi.parseHTMLForLatestVersion(response)
	if err != nil {
		t.Fatalf("parseHTMLForLatestVersion failed: %v", err)
	}

	// Define the expected latest version
	expectedVersion := "6.0.2"

	// Check if the returned latest version is correct
	if latestVersion != expectedVersion {
		t.Errorf("Expected latest version %s, but got %s", expectedVersion, latestVersion)
	}
}

func TestSelectLatestStableVersion(t *testing.T) {
	testCases := []struct {
		name     string
		versions []string
		want     string
		wantErr  bool
	}{
		{
			name:     "prefer stable over beta",
			versions: []string{"1.0.0", "1.1.0b1", "1.0.1"},
			want:     "1.0.1",
		},
		{
			name:     "prefer stable over rc",
			versions: []string{"2.0.0rc1", "1.9.9", "2.0.0rc2"},
			want:     "1.9.9",
		},
		{
			name:     "use highest stable version",
			versions: []string{"1.0.0", "1.1.0", "1.0.1"},
			want:     "1.1.0",
		},
		{
			name:     "fallback to pre-release if no stable",
			versions: []string{"1.0.0b1", "1.0.0b2", "1.0.0rc1"},
			want:     "1.0.0rc1",
		},
		{
			name:     "complex version numbers",
			versions: []string{"1.0.0", "1.0.1alpha", "1.0.1beta", "1.0.1rc1", "1.0.1"},
			want:     "1.0.1",
		},
		{
			name:     "empty input",
			versions: []string{},
			wantErr:  true,
		},
	}

	pypi := New(true)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pypi.selectLatestStableVersion(tc.versions)
			if (err != nil) != tc.wantErr {
				t.Errorf("selectLatestStableVersion() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("selectLatestStableVersion() = %v, want %v", got, tc.want)
			}
		})
	}
}

func BenchmarkStandardJSONProcessing(b *testing.B) {
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
		// Create a new reader for each iteration
		reader := strings.NewReader(jsonResponse)

		// Use the old method
		var data struct {
			Releases map[string]interface{} `json:"releases"`
		}
		err := json.NewDecoder(reader).Decode(&data)
		if err != nil {
			b.Fatal(err)
		}

		// Extract version strings
		var versions []string
		for version := range data.Releases {
			versions = append(versions, version)
		}

		// Ensure we got expected number of versions
		if len(versions) != 5 {
			b.Fatalf("Expected 5 versions, got %d", len(versions))
		}
	}
}

func BenchmarkOptimizedJSONProcessing(b *testing.B) {
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
		// Create a new reader for each iteration
		reader := strings.NewReader(jsonResponse)

		// Use the optimized method
		versions, err := extractVersionsFromPyPIJSON(reader)
		if err != nil {
			b.Fatal(err)
		}

		// Ensure we got expected number of versions
		if len(versions) != 5 {
			b.Fatalf("Expected 5 versions, got %d", len(versions))
		}
	}
}

// Create a larger benchmark for more realistic testing
func BenchmarkLargeJSONProcessingComparison(b *testing.B) {
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
			reader := strings.NewReader(largeJSON)

			var data struct {
				Releases map[string]interface{} `json:"releases"`
			}
			err := json.NewDecoder(reader).Decode(&data)
			if err != nil {
				b.Fatal(err)
			}

			var versions []string
			for version := range data.Releases {
				versions = append(versions, version)
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

			versions, err := extractVersionsFromPyPIJSON(reader)
			if err != nil {
				b.Fatal(err)
			}

			if len(versions) != 100 {
				b.Fatalf("Expected 100 versions, got %d", len(versions))
			}
		}
	})
}
