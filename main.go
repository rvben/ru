package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"

	semv "github.com/Masterminds/semver/v3"
	"golang.org/x/net/html"
	"gopkg.in/ini.v1"
)

// Define the version of the tool
const version = "0.1.12"

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
	flag.Usage = func() {
		fmt.Println("Usage:")
		fmt.Println("  ru update [path]   Update requirements*.txt files in the specified path (default: current directory)")
		fmt.Println("  ru version         Show the version of the tool")
		fmt.Println("  ru help            Show this help message")
	}
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")

	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		return
	}

	// Handle the 'version' command
	if args[0] == "version" {
		fmt.Printf("ru %s\n", version)
		return
	}

	// Handle the 'help' command
	if args[0] == "help" {
		flag.Usage()
		return
	}

	// Handle the 'update' command
	if args[0] == "update" {
		path := "."
		if len(args) > 1 {
			path = args[1]
		}

		verbose = *verboseFlag

		// Set verbose logging flag if needed
		if verbose {
			verboseLog("Starting update process...")
		}

		// Check if ~/.config/pip/pip.conf or /etc/pip.conf exists and use the index-url if defined
		setCustomIndexURL()

		// Start logging (only in verbose mode)
		verboseLog("Starting to process the directory:", path)

		// Walk the directory to find requirements*.txt files
		err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
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
		return
	}

	// If the command is not recognized, show the help message
	flag.Usage()
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

	// Use a map to store unique package lines
	uniqueLines := make(map[string]struct{})
	var originalLines []string
	var sortedLines []string
	modulesUpdatedInFile := 0
	endsWithNewline := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		originalLines = append(originalLines, line)
		if line == "" || strings.HasPrefix(line, "#") {
			// Directly add comments or empty lines
			sortedLines = append(sortedLines, line)
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
			sortedLines = append(sortedLines, line)
			continue
		}

		var updatedLine string
		if versionConstraints != "" {
			// Update directly to the latest version if the constraint is an exact match
			if strings.HasPrefix(versionConstraints, "==") {
				if strings.TrimPrefix(versionConstraints, "==") != latestVersion {
					updatedLine = fmt.Sprintf("%s==%s", packageName, latestVersion)
					modulesUpdatedInFile++
					verboseLog("Updated exact match:", line, "->", updatedLine)
				} else {
					updatedLine = line
				}
			} else if checkVersionConstraints(latestVersion, versionConstraints) {
				verboseLog("Latest version is within the specified range:", latestVersion)
				updatedLine = line
			} else {
				log.Printf("Warning: Latest version %s for package %s is not within the specified range (%s)\n", latestVersion, packageName, versionConstraints)
				updatedLine = line
			}
		} else {
			if !strings.HasSuffix(line, "=="+latestVersion) {
				updatedLine = fmt.Sprintf("%s==%s", packageName, latestVersion)
				modulesUpdatedInFile++
				verboseLog("Updated:", line, "->", updatedLine)
			} else {
				updatedLine = line
			}
		}

		// Add the updated line to the map to ensure uniqueness
		uniqueLines[updatedLine] = struct{}{}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading file:", err)
		return
	}

	// Convert the map to a slice and sort it
	for line := range uniqueLines {
		sortedLines = append(sortedLines, line)
	}
	sort.Strings(sortedLines)

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

	// Join the sorted lines with newlines
	output := strings.Join(sortedLines, "\n")

	// If the original file ended with a newline, ensure the output does too
	if endsWithNewline {
		output += "\n"
	}

	// Check if sorting or updating changed the file
	if modulesUpdatedInFile > 0 || !equalStrings(originalLines, sortedLines) {
		filesUpdated++
		modulesUpdated += modulesUpdatedInFile
	} else {
		filesUnchanged++
	}

	// Write the sorted and unique lines back to the file
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
	var urlString string
	if isCustomIndexURL {
		// Handle custom index URL
		if isCodeArtifact {
			urlString = fmt.Sprintf("%s/%s/", pypiURL, packageName)
		} else {
			urlString = fmt.Sprintf("%s/%s/json", pypiURL, packageName)
		}
	} else {
		// Default PyPI URL
		urlString = fmt.Sprintf("%s/%s/json", pypiURL, packageName)
	}

	// Log the URL if verbose mode is enabled
	verboseLog("Calling URL:", urlString)

	// Parse the original URL to check for user info (username and password)
	originalURL, err := url.Parse(urlString)
	if err != nil {
		log.Println("Error parsing URL:", err)
		return ""
	}
	originalUserInfo := originalURL.User

	// Create an HTTP client with a custom redirect policy
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Log each redirect if verbose mode is enabled
			verboseLog("Redirected to:", req.URL.String())

			// Preserve the credentials from the original request if provided
			if originalUserInfo != nil {
				req.URL.User = originalUserInfo
			}

			// Allow up to 10 redirects
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Make the request
	resp, err := client.Get(urlString)
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
	var versions []*semv.Version

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			// End of the document, return the latest version found
			if len(versions) == 0 {
				return ""
			}

			// Sort the versions using semantic versioning
			sort.Sort(semv.Collection(versions))
			return versions[len(versions)-1].String()

		case tt == html.StartTagToken:
			t := z.Token()

			// Check for <a> tags with a href attribute that contains the version
			if t.Data == "a" {
				for _, a := range t.Attr {
					if a.Key == "href" {
						// Extract the version string from the href attribute
						versionPath := strings.Trim(a.Val, "/")
						parts := strings.Split(versionPath, "/")
						versionStr := parts[0]

						// Parse the version using semver
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

// Helper function to compare version constraints
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
