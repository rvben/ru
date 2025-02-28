package pypi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"gopkg.in/ini.v1"

	"github.com/rvben/ru/internal/cache"
	"github.com/rvben/ru/internal/utils"
)

// Buffer size for JSON token reader
const bufferSize = 4096

// PyPI represents a PyPI package manager
type PyPI struct {
	pypiURL          string
	isCustomIndexURL bool
	isCodeArtifact   bool
	versionCache     map[string]string
	cacheMutex       sync.Mutex
	cache            *cache.Cache
	noCache          bool
	client           *http.Client
}

// New creates a new PyPI package manager
func New(noCache bool) *PyPI {
	// Create a transport with connection pooling
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   10,
	}

	p := &PyPI{
		pypiURL:      "https://pypi.org/pypi",
		versionCache: make(map[string]string),
		noCache:      noCache,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
	if !noCache {
		p.cache = cache.NewCache()
		if err := p.cache.Load(); err != nil {
			utils.VerboseLog("Error loading cache:", err)
		}
	}

	// Try to load custom index URL from pip.conf
	if err := p.SetCustomIndexURL(); err != nil {
		utils.VerboseLog("Error setting custom index URL:", err)
	}

	utils.VerboseLog("Using PyPI URL:", p.pypiURL)
	return p
}

func (p *PyPI) SetCustomIndexURL() error {
	potentialLocations := []string{
		filepath.Join(os.Getenv("HOME"), ".config", "pip", "pip.conf"),
		"/etc/pip.conf",
	}

	for _, configPath := range potentialLocations {
		if _, err := os.Stat(configPath); err == nil {
			utils.VerboseLog("Found pip.conf at", configPath)
			cfg, err := ini.Load(configPath)
			if err != nil {
				return fmt.Errorf("error reading pip.conf: %w", err)
			}

			if indexURL := cfg.Section("global").Key("index-url").String(); indexURL != "" {
				p.pypiURL = strings.TrimSuffix(indexURL, "/")
				p.isCustomIndexURL = true
				p.isCodeArtifact = strings.Contains(p.pypiURL, ".codeartifact.")
				utils.VerboseLog("Using custom index URL from pip.conf:", p.pypiURL)
				return nil
			}
		}
	}

	utils.VerboseLog("No custom index-url found, using default PyPI URL.")
	return nil
}

func (p *PyPI) GetLatestVersion(packageName string) (string, error) {
	// First check if we have a cached version
	if !p.noCache {
		if version, ok := p.cache.Get(packageName); ok {
			utils.VerboseLog("Using cached version for", packageName+":", version)
			return version, nil
		}
	}

	// Try to get version from custom index first
	if p.isCustomIndexURL {
		version, err := p.getLatestVersionFromHTML(packageName)
		if err != nil {
			// Provide more specific error messages for common failures
			if urlErr, ok := err.(*url.Error); ok {
				if urlErr.Timeout() {
					return "", fmt.Errorf("custom index timed out (%s): %w", p.pypiURL, err)
				}
				if _, ok := urlErr.Err.(*net.DNSError); ok {
					return "", fmt.Errorf("custom index not reachable (%s): %w", p.pypiURL, err)
				}
			}
			if strings.Contains(err.Error(), "non-OK status") {
				return "", fmt.Errorf("custom index returned error (%s): %w", p.pypiURL, err)
			}
			// Generic error message for other cases
			return "", fmt.Errorf("custom index error (%s): %w", p.pypiURL, err)
		}
		if version == "" {
			return "", fmt.Errorf("package %s not found in custom index (%s)", packageName, p.pypiURL)
		}
		if !p.noCache {
			p.cache.Set(packageName, version)
			if err := p.cache.Save(); err != nil {
				utils.VerboseLog("Warning: Failed to save cache:", err)
			}
		}
		return version, nil
	}

	// Only fall back to PyPI if no custom index is specified
	if !p.isCustomIndexURL {
		version, err := p.getLatestVersionFromPyPI(packageName)
		if err != nil {
			return "", fmt.Errorf("failed to get version from PyPI for %s: %w", packageName, err)
		}
		if !p.noCache {
			p.cache.Set(packageName, version)
			if err := p.cache.Save(); err != nil {
				utils.VerboseLog("Warning: Failed to save cache:", err)
			}
		}
		return version, nil
	}

	return "", fmt.Errorf("no version found for package %s", packageName)
}

func (p *PyPI) getLatestVersionFromHTML(packageName string) (string, error) {
	packageName = strings.TrimSpace(packageName)
	packageName = strings.ReplaceAll(packageName, ".", "-")
	packageName = strings.ReplaceAll(packageName, "_", "-")
	packageName = strings.ToLower(packageName)

	// Extract base package name if it has extras
	if idx := strings.Index(packageName, "["); idx != -1 {
		packageName = packageName[:idx]
	}

	url := fmt.Sprintf("%s/%s/", p.pypiURL, packageName)

	utils.VerboseLog("Fetching latest version for package:", packageName, "from URL:", url)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version for package %s: %w", packageName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned non-OK status: %s", resp.Status)
	}

	return p.parseHTMLForLatestVersion(resp)
}

func (p *PyPI) getLatestVersionFromPyPI(packageName string) (string, error) {
	packageName = strings.TrimSpace(packageName)
	url := fmt.Sprintf("%s/%s/json", p.pypiURL, packageName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for package %s: %w", packageName, err)
	}
	// Add headers to improve caching
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Connection", "keep-alive")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version for package %s: %w", packageName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned non-OK status: %s", resp.Status)
	}

	// Use buffered reader for efficiency
	bufferedBody := bufio.NewReaderSize(resp.Body, bufferSize)

	// Use standard JSON unmarshaling for PyPI as it's more efficient for this structure
	var data struct {
		Releases map[string]interface{} `json:"releases"`
	}

	if err := json.NewDecoder(bufferedBody).Decode(&data); err != nil {
		return "", fmt.Errorf("error parsing JSON for package %s: %w", packageName, err)
	}

	// Extract versions from the releases map
	var versions []string
	for version := range data.Releases {
		versions = append(versions, version)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found")
	}

	return p.selectLatestStableVersion(versions)
}

func (p *PyPI) parseHTMLForLatestVersion(resp *http.Response) (string, error) {
	var versions []string
	seenVersions := make(map[string]bool) // To prevent duplicates

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			if z.Err() == io.EOF {
				utils.VerboseLog("Found versions:", versions)
				version, err := p.selectLatestStableVersion(versions)
				if err != nil {
					return "", fmt.Errorf("error selecting latest version: %w", err)
				}
				utils.VerboseLog("Selected version:", version)
				return version, nil
			}
			return "", z.Err()

		case tt == html.StartTagToken:
			t := z.Token()
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						versionPath := strings.Trim(a.Val, "/")
						parts := strings.Split(versionPath, "/")
						if len(parts) > 0 {
							version := parts[0]
							// Only add if we haven't seen this version before
							if !seenVersions[version] {
								versions = append(versions, version)
								seenVersions[version] = true
								utils.VerboseLog("Found version:", version, "IsPrerelease:", isPrerelease(version))
							}
						}
					}
				}
			}
		}
	}
}

// compareVersions compares two version strings
func compareVersions(v1, v2 string) int {
	// Split version strings into parts
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int

		// Get number from first version, defaulting to 0 if part doesn't exist
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}

		// Get number from second version, defaulting to 0 if part doesn't exist
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}

		if num1 < num2 {
			return -1
		}
		if num1 > num2 {
			return 1
		}
	}

	return 0
}

// isPrerelease checks if a version string contains pre-release indicators
func isPrerelease(version string) bool {
	prereleaseIndicators := []string{
		"a", "b", "c", "rc", "alpha", "beta", "dev", "preview",
		".dev", ".a", ".b", ".rc", "-a", "-b", "-rc",
	}

	versionLower := strings.ToLower(version)
	for _, indicator := range prereleaseIndicators {
		if strings.Contains(versionLower, indicator) {
			return true
		}
	}
	return false
}

// stripPrereleaseSuffix removes pre-release suffixes for version comparison
func stripPrereleaseSuffix(version string) string {
	re := regexp.MustCompile(`(\.post\d+|\.dev\d+|a\d*|b\d*|c\d*|rc\d*|alpha\d*|beta\d*|preview\d*|[-+].*)$`)
	return re.ReplaceAllString(version, "")
}

func (p *PyPI) selectLatestStableVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found")
	}

	utils.VerboseLog("Selecting from versions:", versions)

	// Separate stable and pre-release versions
	var stableVersions []string
	var preReleaseVersions []string

	for _, version := range versions {
		if isPrerelease(version) {
			utils.VerboseLog("Pre-release version:", version)
			preReleaseVersions = append(preReleaseVersions, version)
		} else {
			utils.VerboseLog("Stable version:", version)
			stableVersions = append(stableVersions, version)
		}
	}

	// If we have stable versions, use those; otherwise fall back to pre-release
	var candidateVersions []string
	if len(stableVersions) > 0 {
		utils.VerboseLog("Using stable versions:", stableVersions)
		candidateVersions = stableVersions
	} else {
		utils.VerboseLog("No stable versions found, using pre-release versions:", preReleaseVersions)
		candidateVersions = preReleaseVersions
	}

	// Sort the candidate versions
	sort.Slice(candidateVersions, func(i, j int) bool {
		return compareVersions(candidateVersions[i], candidateVersions[j]) < 0
	})

	if len(candidateVersions) == 0 {
		return "", fmt.Errorf("no valid versions found")
	}

	result := candidateVersions[len(candidateVersions)-1]
	utils.VerboseLog("Selected version:", result)
	return result, nil
}

func (p *PyPI) CheckEndpoint() error {
	// Try to fetch a known package to verify the endpoint is working
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var testURL string
	if p.isCustomIndexURL {
		// For custom index URLs, try to access the base URL
		testURL = fmt.Sprintf("%s/%s/", p.pypiURL, "pip")
	} else {
		// For PyPI, try to access a known package
		testURL = p.pypiURL + "/pip/json"
	}

	resp, err := client.Get(testURL)
	if err != nil {
		return fmt.Errorf("failed to connect to PyPI endpoint (tried %s): %w", testURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PyPI endpoint (tried %s) returned status code %d", testURL, resp.StatusCode)
	}

	return nil
}

// extractVersionsFromPyPIJSON efficiently extracts only the version keys from PyPI JSON response
// without parsing the entire structure. This method is kept for reference but is less efficient
// than standard unmarshaling for PyPI's structure.
func extractVersionsFromPyPIJSON(r io.Reader) ([]string, error) {
	// Create a buffered reader for efficiency
	br := bufio.NewReaderSize(r, bufferSize)
	dec := json.NewDecoder(br)

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
		if err := skipValue(dec); err != nil {
			return nil, err
		}
	}

	return versions, nil
}

// skipValue skips the next value in the JSON decoder stream
func skipValue(dec *json.Decoder) error {
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
