package pypi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	semv "github.com/Masterminds/semver/v3"
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
	url := fmt.Sprintf("%s/%s/", p.pypiURL, packageName)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // Allow redirects
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for package %s: %w", packageName, err)
	}

	// Parse the URL to extract username and password if present
	parsedURL, err := utils.ParseURL(p.pypiURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse PyPI URL: %w", err)
	}

	// Set basic auth if username and password are provided
	if parsedURL.User != nil {
		username := parsedURL.User.Username()
		password, _ := parsedURL.User.Password()
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req)
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
	var versions []*semv.Version

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			if len(versions) == 0 {
				return "", fmt.Errorf("no versions found")
			}
			sort.Sort(semv.Collection(versions))
			return versions[len(versions)-1].String(), nil

		case tt == html.StartTagToken:
			t := z.Token()
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						versionPath := strings.Trim(a.Val, "/")
						parts := strings.Split(versionPath, "/")
						versionStr := parts[0]
						version, err := semv.NewVersion(versionStr)
						if err == nil {
							versions = append(versions, version)
						}
					}
				}
			}
		}
	}
}
