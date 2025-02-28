package pypi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetLatestVersionGzipped tests the handling of gzip-compressed PyPI responses
func TestGetLatestVersionGzipped(t *testing.T) {
	// Create sample PyPI response JSON
	jsonResponse := `{
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
	}`

	// Create a mock HTTP server that returns gzipped responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip encoding
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			// Respond to both standard format and /json format
			if strings.Contains(r.URL.Path, "/example-package/") || strings.Contains(r.URL.Path, "/example-package/json") {
				// Compress the response
				var buf bytes.Buffer
				gzipWriter := gzip.NewWriter(&buf)
				_, err := gzipWriter.Write([]byte(jsonResponse))
				if err != nil {
					t.Fatalf("Failed to write to gzip writer: %v", err)
				}
				err = gzipWriter.Close()
				if err != nil {
					t.Fatalf("Failed to close gzip writer: %v", err)
				}

				// Set appropriate headers for gzipped content
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Encoding", "gzip")
				w.WriteHeader(http.StatusOK)

				// Write the compressed data
				_, err = w.Write(buf.Bytes())
				if err != nil {
					t.Fatalf("Failed to write response: %v", err)
				}
				return
			}
		} else {
			// If client doesn't accept gzip, return uncompressed response for test coverage
			if strings.Contains(r.URL.Path, "/example-package/") || strings.Contains(r.URL.Path, "/example-package/json") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := io.WriteString(w, jsonResponse)
				if err != nil {
					t.Fatalf("Failed to write response: %v", err)
				}
				return
			}
		}

		// Return 404 for other packages
		w.WriteHeader(http.StatusNotFound)
		_, err := io.WriteString(w, `{"message": "Package not found"}`)
		if err != nil {
			t.Fatalf("Failed to write error response: %v", err)
		}
	}))
	defer server.Close()

	// Create a PyPI instance with our mock server URL
	pypi := New(true)
	pypi.verbose = true // Enable verbose logging for debug output

	// Test getting the latest version directly from the test server
	version, err := pypi.getLatestVersionFromHTML("example-package", server.URL)
	if err != nil {
		t.Fatalf("getLatestVersionFromHTML failed with gzipped response: %v", err)
	}

	// Verify we got the expected version
	expectedVersion := "2.0.0"
	if version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, version)
	}
}

// TestGzipDecompressionError tests error handling for malformed gzip content
func TestGzipDecompressionError(t *testing.T) {
	// Create a mock HTTP server that returns invalid gzipped data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip encoding
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			// Set gzip content encoding header but send invalid gzip data
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)

			// Write invalid gzip data (not actually compressed)
			_, err := w.Write([]byte(`{"info":{"version":"1.0.0"}}`))
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
			return
		}

		// Return 404 for other cases
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create a PyPI instance with our mock server URL
	pypi := New(true)
	pypi.verbose = true // Enable verbose logging for debug output

	// Expect an error when trying to decompress invalid gzip data
	_, err := pypi.getLatestVersionFromHTML("example-package", server.URL)
	if err == nil {
		t.Fatalf("Expected error when decompressing invalid gzip data, got nil")
	}

	// The error should be related to gzip decompression
	if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("Expected gzip-related error, got: %v", err)
	}
}

// TestMixedEncodingResponses tests behavior with different encoding responses
func TestMixedEncodingResponses(t *testing.T) {
	// Define test cases for different encoding scenarios
	testCases := []struct {
		name                string
		setContentEncoding  bool
		contentEncoding     string
		compressResponse    bool
		expectedErrContains string
		expectedVersion     string
	}{
		{
			name:               "Uncompressed response",
			setContentEncoding: false,
			compressResponse:   false,
			expectedVersion:    "2.0.0",
		},
		{
			name:                "Gzip header but uncompressed content",
			setContentEncoding:  true,
			contentEncoding:     "gzip",
			compressResponse:    false,
			expectedErrContains: "gzip", // Should fail with gzip error
		},
		{
			name:               "Properly gzipped content",
			setContentEncoding: true,
			contentEncoding:    "gzip",
			compressResponse:   true,
			expectedVersion:    "2.0.0",
		},
		{
			name:                "Compressed content but no encoding header",
			setContentEncoding:  false,
			compressResponse:    true,
			expectedErrContains: "invalid", // Should fail with JSON parsing error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create sample PyPI response JSON with version info
			jsonResponse := `{
				"info": {
					"name": "example-package",
					"version": "1.0.0"
				},
				"releases": {
					"1.0.0": [],
					"1.1.0": [],
					"2.0.0": []
				}
			}`

			// Create a mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Set content type
				w.Header().Set("Content-Type", "application/json")

				// Set encoding header if specified
				if tc.setContentEncoding {
					w.Header().Set("Content-Encoding", tc.contentEncoding)
				}

				w.WriteHeader(http.StatusOK)

				// Handle compression based on test case
				if tc.compressResponse {
					// Compress the response with gzip
					var buf bytes.Buffer
					gzipWriter := gzip.NewWriter(&buf)
					_, err := gzipWriter.Write([]byte(jsonResponse))
					if err != nil {
						t.Fatalf("Failed to write to gzip writer: %v", err)
					}
					err = gzipWriter.Close()
					if err != nil {
						t.Fatalf("Failed to close gzip writer: %v", err)
					}

					_, err = w.Write(buf.Bytes())
					if err != nil {
						t.Fatalf("Failed to write response: %v", err)
					}
				} else {
					// Send uncompressed response
					_, err := w.Write([]byte(jsonResponse))
					if err != nil {
						t.Fatalf("Failed to write response: %v", err)
					}
				}
			}))
			defer server.Close()

			// Create a PyPI instance
			pypi := New(true)
			pypi.verbose = true // Enable verbose logging for debug

			// Get the latest version directly using the method we're testing
			version, err := pypi.getLatestVersionFromHTML("example-package", server.URL)

			// Check results based on expectations
			if tc.expectedErrContains != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tc.expectedErrContains)
				} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.expectedErrContains)) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.expectedErrContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if version != tc.expectedVersion {
					t.Errorf("Expected version %s, got: %s", tc.expectedVersion, version)
				}
			}
		})
	}
}
