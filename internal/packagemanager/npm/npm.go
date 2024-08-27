package npm

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type NpmPackageManager struct {
	registryURL string
}

func New() *NpmPackageManager {
	return &NpmPackageManager{
		registryURL: "https://registry.npmjs.org",
	}
}

func (n *NpmPackageManager) GetLatestVersion(packageName string) (string, error) {
	url := fmt.Sprintf("%s/%s/latest", n.registryURL, packageName)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch latest version for package %s", packageName)
	}

	var data struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return data.Version, nil
}

func (n *NpmPackageManager) SetCustomIndexURL(url string) {
	n.registryURL = url
}