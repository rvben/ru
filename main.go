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
	"sort"
	"strings"
	"sync"

	"github.com/blang/semver"
	"golang.org/x/net/html"
	"gopkg.in/ini.v1"
)

// Define the version of the tool
const version = "0.1.5"

// Cache to store the latest version of packages
var versionCache = make(map[string]string)
var cacheMutex sync.Mutex

// Verbose logging flag
var verbose bool

// Default PyPI URL
var pypiURL = "https://pypi.org/pypi"

// Flag to indicate whether a custom index-url is being used and its type
var isCustomIndexURL bool
var isCodeArtifact bool

// Counters for summary output
var filesUpdated int
var modulesUpdated int
var filesUnchanged int

func main() {
	// CLI Flags
	path := flag.String("path", ".", "Path to the directory to search for requirements*.txt files")
	help := flag.Bool("help", false, "Show help message")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	// Handle the version command
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("ru %s\n", version)
		return
	}

	// Check if ~/.config/pip/pip.conf or /etc/pip.conf exists and use the index-url if defined
	setCustomIndexURL()

	if *help {
		flag.Usage()
		return
	}

	// Start logging (only in verbose mode)
	verboseLog("Starting to process the directory:", *path)

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
			verboseLog("Found:", filePath)
			updateRequirementsFile(filePath)
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	// Summary output
	if filesUpdated > 0 {
		fmt.Printf("%d file updated and %d modules updated\n", filesUpdated, modulesUpdated)
	} else {
		fmt.Printf("%d files left unchanged\n", filesUnchanged)
	}

	verboseLog("Completed processing.")
}

func setCustomIndexURL() {
	// Define potential locations for pip.conf
	potentialLocations := []string{
		filepath.Join(os.Getenv("HOME"), ".config", "pip", "pip.conf"),
		"/etc/pip.conf",
	}

	for _, configPath := range potentialLocations {
		if _, err := os.Stat(configPath); err == nil {
			// File exists, attempt to load and parse it
			verboseLog("Found pip.conf at", configPath)
			cfg, err := ini.Load(configPath)
			if err != nil {
				log.Println("Error reading pip.conf:", err)
				return
			}

			// Try to get the index-url from the [global] section
			if indexURL := cfg.Section("global").Key("index-url").String(); indexURL != "" {
				pypiURL = strings.TrimSuffix(indexURL, "/")
				isCustomIndexURL = true

				// Check if it's an AWS CodeArtifact repository
				if strings.Contains(pypiURL, ".codeartifact.") {
					isCodeArtifact = true
				}

				verboseLog("Using custom index URL from pip.conf:", pypiURL)
				return
			}
		}
	}

	verboseLog("No custom index-url found, using default PyPI URL.")
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
	modulesUpdatedInFile := 0
	var originalLines []string

	for scanner.Scan() {
		line := scanner.Text()
		originalLines = append(originalLines, line)
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
			// Update directly to the latest version if the constraint is an exact match
			if strings.HasPrefix(versionConstraints, "==") {
				if strings.TrimPrefix(versionConstraints, "==") != latestVersion {
					updatedLine := fmt.Sprintf("%s==%s", packageName, latestVersion)
					updatedLines = append(updatedLines, updatedLine)
					modulesUpdatedInFile++
					verboseLog("Updated exact match:", line, "->", updatedLine)
				} else {
					updatedLines = append(updatedLines, line)
				}
			} else if checkVersionConstraints(latestVersion, versionConstraints) {
				verboseLog("Latest version is within the specified range:", latestVersion)
				updatedLines = append(updatedLines, line)
			} else {
				log.Printf("Warning: Latest version %s for package %s is not within the specified range (%s)\n", latestVersion, packageName, versionConstraints)
				updatedLines = append(updatedLines, line)
			}
		} else {
			if !strings.HasSuffix(line, "=="+latestVersion) {
				updatedLine := fmt.Sprintf("%s==%s", packageName, latestVersion)
				updatedLines = append(updatedLines, updatedLine)
				modulesUpdatedInFile++
				verboseLog("Updated:", line, "->", updatedLine)
			} else {
				updatedLines = append(updatedLines, line)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading file:", err)
		return
	}

	// Sort the lines alphabetically before writing
	sort.Strings(updatedLines)

	// Check if sorting or updating changed the file
	if modulesUpdatedInFile > 0 || !equalStrings(originalLines, updatedLines) {
		filesUpdated++
		modulesUpdated += modulesUpdatedInFile
	} else {
		filesUnchanged++
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

// Helper function to compare two string slices
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

	// If not found in cache, fetch from custom or default PyPI
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
	var url string
	if isCustomIndexURL {
		// Handle custom index URL
		if isCodeArtifact {
			url = fmt.Sprintf("%s/%s/", pypiURL, packageName)
		} else {
			url = fmt.Sprintf("%s/%s/json", pypiURL, packageName)
		}
	} else {
		// Default PyPI URL
		url = fmt.Sprintf("%s/%s/json", pypiURL, packageName)
	}

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

	if isCodeArtifact {
		// Parse the HTML page to extract the latest version
		return parseHTMLForLatestVersion(resp)
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

func parseHTMLForLatestVersion(resp *http.Response) string {
	// Parse the HTML to find the latest version
	z := html.NewTokenizer(resp.Body)
	var latestVersion string

	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			return latestVersion
		case tt == html.StartTagToken:
			t := z.Token()

			// Check for <a> tags with a href attribute that contains the version
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						// The version is typically the text in the href attribute like "/packages/1.2.3/"
						parts := strings.Split(strings.Trim(a.Val, "/"), "/")
						if len(parts) > 0 {
							version := parts[len(parts)-1]
							if version > latestVersion {
								latestVersion = version
							}
						}
					}
				}
			}
		}
	}
}

func checkVersionConstraints(latestVersion, versionConstraints string) bool {
	if strings.HasPrefix(versionConstraints, "==") {
		// Handle exact version match with "=="
		return latestVersion == strings.TrimPrefix(versionConstraints, "==")
	}

	// Split constraints by comma
	constraintsList := strings.Split(versionConstraints, ",")
	for _, constraint := range constraintsList {
		parsedConstraint, err := semver.ParseRange(strings.TrimSpace(constraint))
		if err != nil {
			log.Println("Error parsing version constraints:", err)
			return false
		}

		latestSemVer, err := semver.ParseTolerant(latestVersion)
		if err != nil {
			log.Println("Error parsing latest version:", err)
			return false
		}

		if !parsedConstraint(latestSemVer) {
			return false
		}
	}

	return true
}

// verboseLog prints log messages only if verbose mode is enabled
func verboseLog(v ...interface{}) {
	if verbose {
		log.Println(v...)
	}
}
