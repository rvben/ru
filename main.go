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

	"github.com/blang/semver/v4"
)

// Cache to store the latest version of packages
var versionCache = make(map[string]string)
var cacheMutex sync.Mutex

// Verbose logging flag
var verbose bool

func main() {
	// CLI Flags
	path := flag.String("path", ".", "Path to the directory to search for requirements*.txt files")
	help := flag.Bool("help", false, "Show help message")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
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

	// This variable will track whether the file ends with a newline
	endsWithNewline := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			updatedLines = append(updatedLines, line)
			continue
		}

		// Extract the package name and version constraints using regex
		re := regexp.MustCompile(`^([a-zA-Z0-9-_]+)([<>=!~]+.*)?`)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			log.Println("Invalid line format:", line)
			continue
		}

		packageName := matches[1]
		versionConstraints := matches[2]
		verboseLog("Processing package:", packageName)

		latestVersion := getCachedLatestVersion(packageName)
		if latestVersion == "" {
			log.Println("Failed to get latest version for package:", packageName)
			updatedLines = append(updatedLines, line)
			continue
		}

		if versionConstraints != "" {
			if isVersionInRange(latestVersion, versionConstraints) {
				verboseLog("Latest version is within the specified range:", latestVersion)
				updatedLines = append(updatedLines, line)
			} else {
				log.Printf("Warning: Latest version %s for package %s is not within the specified range (%s)\n", latestVersion, packageName, versionConstraints)
				updatedLines = append(updatedLines, line)
			}
		} else {
			updatedLine := fmt.Sprintf("%s==%s", packageName, latestVersion)
			updatedLines = append(updatedLines, updatedLine)
			verboseLog("Updated:", line, "->", updatedLine)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading file:", err)
		return
	}

	// Check if the original file ends with a newline
	fileInfo, err := file.Stat()
	if err == nil {
		fileSize := fileInfo.Size()
		if fileSize > 0 {
			// Seek to the last byte of the file
			file.Seek(fileSize-1, 0)
			buffer := make([]byte, 1)
			file.Read(buffer)
			if buffer[0] == '\n' {
				endsWithNewline = true
			}
		}
	}

	// Join the updated lines with newlines
	output := strings.Join(updatedLines, "\n")

	// If the original file ended with a newline, ensure the output does too
	if endsWithNewline {
		output += "\n"
	}

	// Write the updated lines back to the file
	err = os.WriteFile(filePath, []byte(output), 0644)
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
		verboseLog("Cache hit for package:", packageName, "version:", version)
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

func isVersionInRange(latestVersion, versionConstraints string) bool {
	constraints, err := semver.ParseRange(versionConstraints)
	if err != nil {
		log.Println("Error parsing version constraints:", err)
		return false
	}

	latestSemVer, err := semver.ParseTolerant(latestVersion)
	if err != nil {
		log.Println("Error parsing latest version:", err)
		return false
	}

	return constraints(latestSemVer)
}

// verboseLog prints log messages only if verbose mode is enabled
func verboseLog(v ...interface{}) {
	if verbose {
		log.Println(v...)
	}
}
