package npm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	bufferSize         = 4096
	connectionTimeout  = 30 * time.Second
	responseTimeout    = 30 * time.Second
	maxIdleConnections = 10
)

type NPM struct {
	registryURL string
	client      *http.Client
}

func New() *NPM {
	// Create a transport with connection pooling
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   connectionTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   maxIdleConnections,
	}

	return &NPM{
		registryURL: "https://registry.npmjs.org",
		client: &http.Client{
			Transport: transport,
			Timeout:   responseTimeout,
		},
	}
}

func (n *NPM) GetLatestVersion(packageName string) (string, error) {
	url := fmt.Sprintf("%s/%s/latest", n.registryURL, packageName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for package %s: %w", packageName, err)
	}
	// Add headers to improve caching and efficiency
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Connection", "keep-alive")

	resp, err := n.client.Do(req)
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
