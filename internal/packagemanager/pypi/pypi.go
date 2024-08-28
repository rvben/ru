package pypi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/html"
	"gopkg.in/ini.v1"

	"github.com/rvben/ru/internal/cache"
	"github.com/rvben/ru/internal/utils"
)

type PyPI struct {
	pypiURL          string
	isCustomIndexURL bool
	isCodeArtifact   bool
	versionCache     map[string]string
	cacheMutex       sync.Mutex
	cache            *cache.Cache
	noCache          bool
}

func New(noCache bool) *PyPI {
	p := &PyPI{
		pypiURL:      "https://pypi.org/pypi",
		versionCache: make(map[string]string),
		noCache:      noCache,
	}
	if !noCache {
		p.cache = cache.NewCache()
		if err := p.cache.Load(); err != nil {
			utils.VerboseLog("Error loading cache:", err)
		}
	}
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
	if !p.noCache {
		if version, found := p.cache.Get(packageName); found {
			utils.VerboseLog("Cache hit for package:", packageName, "version:", version)
			return version, nil
		}
	}

	var version string
	var err error

	if p.isCodeArtifact {
		version, err = p.getLatestVersionFromHTML(packageName)
	} else {
		version, err = p.getLatestVersionFromPyPI(packageName)
	}

	if err != nil {
		return "", err
	}

	if version != "" && !p.noCache {
		p.cache.Set(packageName, version)
		if err := p.cache.Save(); err != nil {
			utils.VerboseLog("Error saving cache:", err)
		}
	}

	return version, nil
}

func (p *PyPI) getLatestVersionFromHTML(packageName string) (string, error) {
	packageName = strings.TrimSpace(packageName)
	packageName = strings.ReplaceAll(packageName, ".", "-")
	packageName = strings.ToLower(packageName)

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
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version for package %s: %w", packageName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned non-OK status: %s", resp.Status)
	}

	var data struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("error decoding JSON for package %s: %w", packageName, err)
	}

	return data.Info.Version, nil
}

func (p *PyPI) parseHTMLForLatestVersion(resp *http.Response) (string, error) {
	var versions []string
	originalVersions := make(map[string]string)

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			if len(versions) == 0 {
				return "", fmt.Errorf("no versions found")
			}
			sort.Slice(versions, func(i, j int) bool {
				return compareVersions(versions[i], versions[j]) < 0
			})
			latestVersion := versions[len(versions)-1]
			return originalVersions[latestVersion], nil

		case tt == html.StartTagToken:
			t := z.Token()
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						versionPath := strings.Trim(a.Val, "/")
						parts := strings.Split(versionPath, "/")
						versionStr := parts[0]

						// Use regex to strip unwanted suffixes for comparison
						re := regexp.MustCompile(`(\.post\d+|\.dev\d+|a\d*|b\d*|rc\d*|[-+].*)$`)
						parsedVersionStr := re.ReplaceAllString(versionStr, "")

						versions = append(versions, parsedVersionStr)
						originalVersions[parsedVersionStr] = versionStr
					}
				}
			}
		}
	}
}

func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			p1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			p2, _ = strconv.Atoi(parts2[i])
		}

		if p1 < p2 {
			return -1
		} else if p1 > p2 {
			return 1
		}
	}

	return 0
}
