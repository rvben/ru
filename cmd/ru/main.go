package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/rvben/ru/internal/cache"
	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// version is the current version of the tool
const version = "0.1.53"

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

	// Use go install to update the binary
	cmd := exec.Command("go", "install", "github.com/rvben/ru/cmd/ru@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update ru: %w", err)
	}

	fmt.Printf("Successfully updated ru to version %s\n", latestVersion)
	return nil
}

func main() {
	// CLI Flags
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	noCacheFlag := flag.Bool("no-cache", false, "Disable caching")

	// Custom flag parsing
	args := os.Args[1:]
	command := ""
	for i, arg := range args {
		if arg == "update" || arg == "version" || arg == "help" || arg == "clean-cache" || arg == "self-update" {
			command = arg
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	flag.CommandLine.Parse(args)

	// Set verbose mode
	utils.SetVerbose(*verboseFlag)

	switch command {
	case "update":
		updater := update.New(*noCacheFlag)
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
	case "help":
		fmt.Println("Usage: ru [flags] <command>")
		fmt.Println("\nCommands:")
		fmt.Println("  update       Update dependencies in requirements files")
		fmt.Println("  version      Show version information")
		fmt.Println("  clean-cache  Clean the version cache")
		fmt.Println("  self-update  Update ru to the latest version")
		fmt.Println("  help         Show this help message")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
	default:
		fmt.Println("Unknown command. Use 'ru help' for usage information.")
		os.Exit(1)
	}
}
