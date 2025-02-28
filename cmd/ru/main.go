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
	fmt.Println("Usage: ru [global flags] <command> [command flags] [args]")
	fmt.Println("\nCommands:")
	fmt.Println("  update       Update dependencies in requirements files")
	fmt.Println("  version      Show version information")
	fmt.Println("  clean-cache  Clean the version cache")
	fmt.Println("  self update  Update ru to the latest version")
	fmt.Println("  align        Align package versions with existing versions")
	fmt.Println("  help         Show this help message")

	fmt.Println("\nGlobal flags:")
	globalFlags.PrintDefaults()

	fmt.Println("\nUpdate command flags:")
	updateFlags.PrintDefaults()
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
	globalFlags.Bool("verbose", false, "Enable verbose logging")
	globalFlags.Bool("no-cache", false, "Disable caching")

	// Create a new FlagSet for update command
	updateFlags := flag.NewFlagSet("update", flag.ExitOnError)
	verifyFlag := updateFlags.Bool("verify", false, "Verify dependency compatibility (slower)")

	// Define flags for the default command line parser
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	noCache := flag.Bool("no-cache", false, "Disable caching")
	flag.Parse()

	// Set verbose mode based on the flag value
	utils.SetVerbose(*verbose)

	// Get command from args
	args := flag.Args()
	if len(args) == 0 {
		printHelp(globalFlags, updateFlags)
		os.Exit(1)
	}

	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "update":
		// Parse update-specific flags
		if err := updateFlags.Parse(commandArgs); err != nil {
			log.Fatal(err)
		}
		paths := updateFlags.Args()

		// Log if running in test mode
		if isTestMode() {
			utils.VerboseLog("Running in test mode")
			if cacheDir := getCacheDir(); cacheDir != "" {
				utils.VerboseLog("Using cache directory:", cacheDir)
			}
		}

		updater := update.New(*noCache, *verifyFlag, paths)
		if err := updater.Run(); err != nil {
			if !strings.Contains(err.Error(), "dependency verification failed for") {
				log.Fatal(err)
			}
			os.Exit(1)
		}
	case "version":
		fmt.Printf("ru version %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	case "clean-cache":
		if err := cache.Clean(); err != nil {
			log.Fatalf("Failed to clean cache: %v", err)
		}
		fmt.Println("Cache cleaned successfully")
	case "self":
		if len(commandArgs) == 0 {
			fmt.Println("Usage: ru self <command>")
			fmt.Println("\nCommands:")
			fmt.Println("  update  Update ru to the latest version")
			os.Exit(1)
		}

		switch commandArgs[0] {
		case "update":
			if err := selfUpdate(); err != nil {
				log.Fatalf("Failed to self-update: %v", err)
			}
		default:
			fmt.Printf("Unknown self command '%s'. Use 'ru self update' to update ru.\n", commandArgs[0])
			os.Exit(1)
		}
	case "align":
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
