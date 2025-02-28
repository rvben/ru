package npm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rvben/ru/internal/utils"
)

const (
	bufferSize = 4096
)

type NPM struct {
	registryURL string
	client      *utils.OptimizerHTTPClient
}

func New() *NPM {
	return &NPM{
		registryURL: "https://registry.npmjs.org",
		client:      utils.NewHTTPClient(),
	}
}

func (n *NPM) GetLatestVersion(packageName string) (string, error) {
	url := fmt.Sprintf("%s/%s/latest", n.registryURL, packageName)

	// Use optimized HTTP client with retry and circuit breaker
	resp, err := n.client.GetWithRetry(url, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version for package %s: %w", packageName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest version for package %s: status %s", packageName, resp.Status)
	}

	// Use optimized JSON extraction instead of the standard decoder
	version, err := extractVersionFromNPMJSON(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error processing JSON for package %s: %w", packageName, err)
	}

	return version, nil
}

// extractVersionFromNPMJSON efficiently extracts only the version field from NPM JSON response
func extractVersionFromNPMJSON(r io.Reader) (string, error) {
	// Create a buffered reader for efficiency
	br := bufio.NewReaderSize(r, bufferSize)

	dec := json.NewDecoder(br)

	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Look for the version field
		if key, ok := t.(string); ok && key == "version" {
			// Next token should be the version value
			t, err := dec.Token()
			if err != nil {
				return "", err
			}

			// Extract the version value
			if versionVal, ok := t.(string); ok {
				return versionVal, nil
			}

			return "", fmt.Errorf("expected string value for version, got %T", t)
		}
	}

	return "", fmt.Errorf("version field not found in NPM response")
}

func (n *NPM) SetCustomIndexURL(url string) {
	n.registryURL = url
}

// GetRequestMetrics returns metrics about HTTP requests
func (n *NPM) GetRequestMetrics() utils.HTTPClientMetrics {
	return n.client.GetMetrics()
}
