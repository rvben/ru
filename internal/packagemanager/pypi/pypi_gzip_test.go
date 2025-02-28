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
			// Check if the request path contains the expected package name
			if strings.Contains(r.URL.Path, "/example-package/") {
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
			if strings.Contains(r.URL.Path, "/example-package/") {
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
	pypi.pypiURL = server.URL

	// Test getting the latest version from a gzipped response
	version, err := pypi.GetLatestVersion("example-package")
	if err != nil {
		t.Fatalf("GetLatestVersion failed with gzipped response: %v", err)
	}

	// Verify we got the expected version
	expectedVersion := "2.0.0"
	if version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, version)
	}
}

// TestGzipDecompressionError tests handling of malformed gzip data
func TestGzipDecompressionError(t *testing.T) {
	// Create a mock HTTP server that returns invalid gzipped data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid gzip data
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("this is not valid gzip data"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	// Create a PyPI instance with our mock server URL
	pypi := New(true)
	pypi.pypiURL = server.URL

	// Test getting the latest version from invalid gzipped data
	_, err := pypi.GetLatestVersion("example-package")
	if err == nil {
		t.Error("Expected an error for invalid gzip data, got nil")
	} else if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("Expected error message to contain 'gzip', got: %v", err)
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
	}{
		{
			name:               "Uncompressed response",
			setContentEncoding: false,
			compressResponse:   false,
		},
		{
			name:                "Gzip header but uncompressed content",
			setContentEncoding:  true,
			contentEncoding:     "gzip",
			compressResponse:    false,
			expectedErrContains: "gzip", // Should fail with gzip error
		},
		{
			name:               "Incorrect encoding header",
			setContentEncoding: true,
			contentEncoding:    "deflate", // We only handle gzip
			compressResponse:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create sample PyPI response JSON
			jsonResponse := `{
				"info": {
					"name": "example-package",
					"version": "1.0.0"
				},
				"releases": {
					"1.0.0": []
				}
			}`

			// Create a mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				if tc.setContentEncoding {
					w.Header().Set("Content-Encoding", tc.contentEncoding)
				}

				w.WriteHeader(http.StatusOK)

				if tc.compressResponse {
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

					_, err = w.Write(buf.Bytes())
					if err != nil {
						t.Fatalf("Failed to write response: %v", err)
					}
				} else {
					_, err := io.WriteString(w, jsonResponse)
					if err != nil {
						t.Fatalf("Failed to write response: %v", err)
					}
				}
			}))
			defer server.Close()

			// Create a PyPI instance with our mock server URL
			pypi := New(true)
			pypi.pypiURL = server.URL

			// Test getting the latest version
			_, err := pypi.GetLatestVersion("example-package")

			// Verify error behavior matches expectations
			if tc.expectedErrContains != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tc.expectedErrContains)
				} else if !strings.Contains(err.Error(), tc.expectedErrContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.expectedErrContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}
