package pypi

import (
	"encoding/json"
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
	// Expected latest version in the mock JSON response
	expectedVersion := "3.1.0"

	// Create a mock JSON response
	mockResponse := MockPyPIResponse{}
	mockResponse.Info.Version = expectedVersion
	responseData, err := json.Marshal(mockResponse)
	if err != nil {
		t.Fatalf("Failed to marshal mock response: %v", err)
	}

	// Create a test server to serve the mock JSON response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(responseData)
	}))
	defer ts.Close()

	// Create a PyPI instance with the test server URL
	pypi := New()
	pypi.pypiURL = ts.URL

	// Call the function to test
	latestVersion, err := pypi.GetLatestVersion("example-package")
	if err != nil {
		t.Fatalf("GetLatestVersion failed: %v", err)
	}

	// Check if the returned latest version is correct
	if latestVersion != expectedVersion {
		t.Errorf("Expected latest version %s, but got %s", expectedVersion, latestVersion)
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
	pypi := New()

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
