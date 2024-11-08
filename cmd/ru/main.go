package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/rvben/ru/internal/cache"
	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// version is set during build
var version = "dev" // Default to "dev" for development builds

type GithubRelease struct {
	TagName string `json:"tag_name"`
}

func checkLatestVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/rvben/ru/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check latest version: %w", err)
	}
	defer resp.Body.Close()

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

func selfUpdate() error {
	latestVersion, err := checkLatestVersion()
	if err != nil {
		return err
	}

	if version == latestVersion {
		fmt.Println("ru is already at the latest version", version)
		return nil
	}

	fmt.Printf("Updating ru from %s to %s...\n", version, latestVersion)

	// Get the binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Construct download URL for the latest release
	arch := runtime.GOARCH
	goos := runtime.GOOS
	binaryName := "ru"
	if goos == "windows" {
		binaryName += ".exe"
	}

	downloadURL := fmt.Sprintf(
		"https://github.com/rvben/ru/releases/download/v%s/ru_%s_%s",
		latestVersion,
		goos,
		arch,
	)

	// For debugging
	utils.VerboseLog("Downloading from:", downloadURL)

	// Download the new binary
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download update, status: %s", resp.Status)
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "ru-update")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Copy the downloaded binary to temporary file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Make the temporary file executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make temporary file executable: %w", err)
	}

	// Replace the old binary
	if err := os.Rename(tmpFile.Name(), execPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	fmt.Printf("Successfully updated ru to version %s\n", latestVersion)
	return nil
}

func main() {
	// CLI Flags
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	noCacheFlag := flag.Bool("no-cache", false, "Disable caching")

	// Parse flags first
	flag.Parse()

	// Set verbose mode
	utils.SetVerbose(*verboseFlag)

	// Get remaining args after flags
	args := flag.Args()

	// Get command from remaining args
	command := ""
	paths := []string{}

	if len(args) > 0 {
		command = args[0]
		paths = args[1:] // Everything after command
	}

	switch command {
	case "update":
		updater := update.New(*noCacheFlag, paths)
		if err := updater.Run(); err != nil {
			log.Fatal(err)
		}
	case "version":
		fmt.Printf("ru version %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	case "clean-cache":
		if err := cache.Clean(); err != nil {
			log.Fatalf("Failed to clean cache: %v", err)
		}
		fmt.Println("Cache cleaned successfully")
	case "self-update":
		if err := selfUpdate(); err != nil {
			log.Fatalf("Failed to self-update: %v", err)
		}
	case "align":
		aligner := update.NewAligner()
		if err := aligner.Run(); err != nil {
			log.Fatal(err)
		}
	case "help", "":
		fmt.Println("Usage: ru [flags] <command> [args]")
		fmt.Println("\nCommands:")
		fmt.Println("  update       Update dependencies in requirements files")
		fmt.Println("  version      Show version information")
		fmt.Println("  clean-cache  Clean the version cache")
		fmt.Println("  self-update  Update ru to the latest version")
		fmt.Println("  align        Align package versions with existing versions")
		fmt.Println("  help         Show this help message")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
	default:
		fmt.Println("Unknown command. Use 'ru help' for usage information.")
		os.Exit(1)
	}
}
