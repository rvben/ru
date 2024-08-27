package npm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
