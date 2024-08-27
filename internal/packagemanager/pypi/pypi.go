package pypi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
		p.cache.Load()
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

	version, err := p.getLatestVersionFromPyPI(packageName)
	if err != nil {
		return "", err
	}

	if version != "" && !p.noCache {
		p.cache.Set(packageName, version)
		p.cache.Save()
	}

	return version, nil
}

func (p *PyPI) getLatestVersionFromPyPI(packageName string) (string, error) {
	var urlString string
	if p.isCustomIndexURL {
		if p.isCodeArtifact {
			urlString = fmt.Sprintf("%s/%s/", p.pypiURL, packageName)
		} else {
			urlString = fmt.Sprintf("%s/%s/json", p.pypiURL, packageName)
		}
	} else {
		urlString = fmt.Sprintf("%s/%s/json", p.pypiURL, packageName)
	}

	utils.VerboseLog("Calling URL:", urlString)

	originalURL, err := url.Parse(urlString)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %w", err)
	}
	originalUserInfo := originalURL.User

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			utils.VerboseLog("Redirected to:", req.URL.String())
			if originalUserInfo != nil {
				req.URL.User = originalUserInfo
			}
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	resp, err := client.Get(urlString)
	if err != nil {
		return "", fmt.Errorf("error fetching version from PyPI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned non-OK status: %s", resp.Status)
	}

	if p.isCodeArtifact {
		return p.parseHTMLForLatestVersion(resp)
	}

	var versionInfo struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	err = json.NewDecoder(resp.Body).Decode(&versionInfo)
	if err != nil {
		return "", fmt.Errorf("error decoding JSON response: %w", err)
	}

	return versionInfo.Info.Version, nil
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
