package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

	currentVersion := strings.TrimPrefix(version, "v")
	if currentVersion == latestVersion {
		fmt.Println("ru is already at the latest version", version)
		return nil
	}

	fmt.Printf("Updating ru from %s to %s...\n", currentVersion, latestVersion)

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

func printHelp(globalFlags *flag.FlagSet, updateFlags *flag.FlagSet) {
	fmt.Println("Usage: ru <command> [command flags] [args]")
	fmt.Println("\nCommands:")
	fmt.Println("  update       Update dependencies in requirements files")
	fmt.Println("  version      Show version information")
	fmt.Println("  clean-cache  Clean the version cache")
	fmt.Println("  self update  Update ru to the latest version")
	fmt.Println("  align        Align package versions with existing versions")
	fmt.Println("  help         Show this help message")

	fmt.Println("\nGlobal flags (available for all commands):")
	globalFlags.PrintDefaults()

	fmt.Println("\nUpdate command flags (use after 'update' command):")
	updateFlags.PrintDefaults()

	fmt.Println("\nExamples:")
	fmt.Println("  ru update                     Update all dependencies")
	fmt.Println("  ru update -verify             Update with verification")
	fmt.Println("  ru update -verbose            Update with verbose logging")
	fmt.Println("  ru update -no-cache           Update without using cache")
	fmt.Println("  ru update -verbose -verify    Combine multiple flags")
	fmt.Println("  ru clean-cache -verbose       Use global flags with other commands")
}

// isTestMode returns true if we're running in test mode
func isTestMode() bool {
	return os.Getenv("RU_TEST_MODE") == "1"
}

// getCacheDir returns the cache directory, which can be overridden in test mode
func getCacheDir() string {
	if dir := os.Getenv("RU_CACHE_DIR"); dir != "" {
		return dir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return ".ru/cache"
	}
	return filepath.Join(homeDir, ".ru", "cache")
}

func main() {
	// Create a new FlagSet for global flags
	globalFlags := flag.NewFlagSet("global", flag.ExitOnError)
	verboseFlag := globalFlags.Bool("verbose", false, "Enable verbose logging")
	noColorFlag := globalFlags.Bool("no-color", false, "Disable colored output")
	// Only declare noCacheFlag if we're going to use it
	// noCacheFlag := globalFlags.Bool("no-cache", false, "Disable caching")
	globalFlags.Bool("no-cache", false, "Disable caching")

	// Create a new FlagSet for update command
	updateFlags := flag.NewFlagSet("update", flag.ExitOnError)
	verifyFlag := updateFlags.Bool("verify", false, "Verify dependency compatibility (slower)")
	dryRunFlag := updateFlags.Bool("dry-run", false, "Show what would be updated without making changes")
	// Add the global flags to the update command as well
	updateVerboseFlag := updateFlags.Bool("verbose", false, "Enable verbose logging")
	updateNoCacheFlag := updateFlags.Bool("no-cache", false, "Disable caching")
	updateNoColorFlag := updateFlags.Bool("no-color", false, "Disable colored output")

	// Check if any args were provided
	if len(os.Args) < 2 {
		printHelp(globalFlags, updateFlags)
		os.Exit(1)
	}

	// Get the command
	command := os.Args[1]

	switch command {
	case "update":
		// Parse update-specific flags
		if err := updateFlags.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		paths := updateFlags.Args()

		// Set verbose mode
		utils.SetVerbose(*updateVerboseFlag)

		// Set color mode
		if *updateNoColorFlag {
			utils.DisableColors()
		}

		// Log if running in test mode
		testMode := isTestMode()
		if testMode {
			utils.Info("system", "Running in test mode")
			if cacheDir := getCacheDir(); cacheDir != "" {
				utils.Info("system", "Using cache directory: %s", cacheDir)
			}
		}

		// Create updater
		updater := update.New(*updateNoCacheFlag, *verifyFlag, paths)

		// Set dry run mode if flag is provided
		if *dryRunFlag {
			updater.SetDryRun(true)
		}

		// Run the updater
		if err := updater.Run(); err != nil {
			utils.Error("Update failed: %v", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("ru version %s\n", version)
	case "clean-cache":
		// Parse global flags
		if err := globalFlags.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}

		// Set verbose mode
		utils.SetVerbose(*verboseFlag)

		// Set color mode
		if *noColorFlag {
			utils.DisableColors()
		}

		// Clean the cache
		if err := cache.Clean(); err != nil {
			utils.Error("Failed to clean cache: %v", err)
			os.Exit(1)
		}
		utils.Success("Cache cleaned successfully")
	case "self":
		// Check if we have a subcommand
		if len(os.Args) < 3 {
			printHelp(globalFlags, updateFlags)
			os.Exit(1)
		}

		// Parse global flags
		if err := globalFlags.Parse(os.Args[3:]); err != nil {
			log.Fatal(err)
		}

		// Set verbose mode
		utils.SetVerbose(*verboseFlag)

		// Set color mode
		if *noColorFlag {
			utils.DisableColors()
		}

		// Handle self update
		if os.Args[2] == "update" {
			if err := selfUpdate(); err != nil {
				utils.Error("Self update failed: %v", err)
				os.Exit(1)
			}
		} else {
			printHelp(globalFlags, updateFlags)
			os.Exit(1)
		}
	case "align":
		if err := globalFlags.Parse(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		utils.SetVerbose(*verboseFlag)
		aligner := update.NewAligner()
		if err := aligner.Run(); err != nil {
			log.Fatal(err)
		}
	case "help", "":
		printHelp(globalFlags, updateFlags)
	default:
		fmt.Println("Unknown command. Use 'ru help' for usage information.")
		os.Exit(1)
	}
}
