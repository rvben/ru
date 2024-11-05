package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rvben/ru/internal/cache"
	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// version is the current version of the tool
const version = "0.1.51"

func main() {
	// CLI Flags
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	noCacheFlag := flag.Bool("no-cache", false, "Disable caching")

	// Custom flag parsing
	args := os.Args[1:]
	command := ""
	for i, arg := range args {
		if arg == "update" || arg == "version" || arg == "help" || arg == "clean-cache" {
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
		fmt.Printf("ru version %s\n", version)
	case "clean-cache":
		if err := cache.Clean(); err != nil {
			log.Fatalf("Failed to clean cache: %v", err)
		}
		fmt.Println("Cache cleaned successfully")
	case "help":
		fmt.Println("Usage: ru [flags] <command>")
		fmt.Println("\nCommands:")
		fmt.Println("  update       Update dependencies in requirements files")
		fmt.Println("  version      Show version information")
		fmt.Println("  clean-cache  Clean the version cache")
		fmt.Println("  help         Show this help message")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
	default:
		fmt.Println("Unknown command. Use 'ru help' for usage information.")
		os.Exit(1)
	}
}
