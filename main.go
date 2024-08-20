package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Cache to store the latest version of packages
var versionCache = make(map[string]string)
var cacheMutex sync.Mutex

func main() {
	// CLI Flags
	path := flag.String("path", ".", "Path to the directory to search for requirements*.txt files")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	// Start logging
	log.Println("Starting to process the directory:", *path)

	// Walk the directory to find requirements*.txt files
	err := filepath.Walk(*path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Match files with the pattern requirements*.txt
		matched, err := filepath.Match("requirements*.txt", filepath.Base(filePath))
		if err != nil {
			return err
		}
		if matched {
			log.Println("Found:", filePath)
			updateRequirementsFile(filePath)
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	log.Println("Completed processing.")
}

func updateRequirementsFile(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	var updatedLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			updatedLines = append(updatedLines, line)
			continue
		}

		// Extract the package name using regex
		re := regexp.MustCompile(`^([a-zA-Z0-9-_]+)([<>=!~]*)(.*)`)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			log.Println("Invalid line format:", line)
			continue
		}

		packageName := matches[1]
		log.Println("Processing package:", packageName)

		latestVersion := getCachedLatestVersion(packageName)
		if latestVersion == "" {
			log.Println("Failed to get latest version for package:", packageName)
			updatedLines = append(updatedLines, line)
			continue
		}

		updatedLine := fmt.Sprintf("%s==%s", packageName, latestVersion)
		updatedLines = append(updatedLines, updatedLine)
		log.Println("Updated:", line, "->", updatedLine)
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading file:", err)
		return
	}

	// Write the updated lines back to the file
	err = os.WriteFile(filePath, []byte(strings.Join(updatedLines, "\n")), 0644)
	if err != nil {
		log.Println("Error writing updated file:", err)
	}
}

func getCachedLatestVersion(packageName string) string {
	// Check the cache first
	cacheMutex.Lock()
	version, found := versionCache[packageName]
	cacheMutex.Unlock()

	if found {
		log.Println("Cache hit for package:", packageName, "version:", version)
		return version
	}

	// If not found in cache, fetch from PyPI
	version = getLatestVersionFromPyPI(packageName)

	// Cache the result
	if version != "" {
		cacheMutex.Lock()
		versionCache[packageName] = version
		cacheMutex.Unlock()
	}

	return version
}

func getLatestVersionFromPyPI(packageName string) string {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", packageName)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching version from PyPI:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("PyPI returned non-OK status:", resp.Status)
		return ""
	}

	type VersionInfo struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	var versionInfo VersionInfo
	err = json.NewDecoder(resp.Body).Decode(&versionInfo)
	if err != nil {
		log.Println("Error decoding JSON response:", err)
		return ""
	}

	return versionInfo.Info.Version
}
