package npm

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetLatestVersionGzipped tests the handling of gzip-compressed NPM responses
func TestGetLatestVersionGzipped(t *testing.T) {
	// Create sample NPM response JSON
	jsonResponse := `{
		"name": "example-package",
		"version": "2.0.0",
		"description": "A test package",
		"dependencies": {
			"dependency1": "^1.0.0",
			"dependency2": "^2.0.0"
		}
	}`

	// Create a mock HTTP server that returns gzipped responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip encoding
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
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
		} else {
			// If client doesn't accept gzip, return uncompressed response for test coverage
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := io.WriteString(w, jsonResponse)
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
		}
	}))
	defer server.Close()

	// Create an NPM instance with our mock server URL
	npm := New()
	npm.registryURL = server.URL

	// Test getting the latest version from a gzipped response
	version, err := npm.GetLatestVersion("example-package")
	if err != nil {
		t.Fatalf("GetLatestVersion failed with gzipped response: %v", err)
	}

	// Verify we got the expected version
	expectedVersion := "2.0.0"
	if version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, version)
	}
}

// TestNPMGzipDecompressionError tests handling of malformed gzip data
func TestNPMGzipDecompressionError(t *testing.T) {
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

	// Create an NPM instance with our mock server URL
	npm := New()
	npm.registryURL = server.URL

	// Test getting the latest version from invalid gzipped data
	_, err := npm.GetLatestVersion("example-package")
	if err == nil {
		t.Error("Expected an error for invalid gzip data, got nil")
	} else if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("Expected error message to contain 'gzip', got: %v", err)
	}
}

// TestNPMMixedEncodingResponses tests behavior with different encoding responses
func TestNPMMixedEncodingResponses(t *testing.T) {
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
			// Create sample NPM response JSON
			jsonResponse := `{
				"name": "example-package",
				"version": "1.0.0"
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

			// Create an NPM instance with our mock server URL
			npm := New()
			npm.registryURL = server.URL

			// Test getting the latest version
			_, err := npm.GetLatestVersion("example-package")

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
