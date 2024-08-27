package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rvben/ru/internal/packagemanager/pypi"
	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// version is the current version of the tool
const version = "0.1.28"

func main() {
	// CLI Flags
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	noCacheFlag := flag.Bool("no-cache", false, "Disable caching")

	// Custom flag parsing
	args := os.Args[1:]
	command := ""
	for i, arg := range args {
		if arg == "update" || arg == "version" || arg == "help" {
			command = arg
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	flag.CommandLine.Parse(args)

	// Custom usage function
	flag.Usage = func() {
		fmt.Println("Usage:")
		fmt.Println("  ru [command] [flags] [arguments]")
		fmt.Println("\nCommands:")
		fmt.Println("  update [path]   Update requirements*.txt and package.json files in the specified path (default: current directory)")
		fmt.Println("  version         Show the version of the tool")
		fmt.Println("  help            Show this help message")
		fmt.Println("\nFlags:")
		fmt.Println("  -verbose        Enable verbose logging")
		fmt.Println("  -no-cache       Disable caching")
		fmt.Println("\nExamples:")
		fmt.Println("  ru update          Update requirements*.txt and package.json files in the current directory")
		fmt.Println("  ru update /path/to/dir  Update requirements*.txt and package.json files in the specified directory")
		fmt.Println("  ru -verbose update /path/to/dir  Update with verbose logging")
		fmt.Println("  ru update -verbose /path/to/dir  Update with verbose logging")
	}

	if command == "" {
		flag.Usage()
		return
	}

	utils.SetVerbose(*verboseFlag)

	switch command {
	case "version":
		fmt.Printf("ru %s\n", version)
	case "help":
		flag.Usage()
	case "update":
		path := "."
		if len(flag.Args()) > 0 {
			path = flag.Args()[0]
		}
		pm := pypi.New(*noCacheFlag)
		if err := pm.SetCustomIndexURL(); err != nil {
			log.Fatalf("Error setting custom index URL: %v", err)
		}
		updater := update.NewUpdater(pm)
		if err := updater.ProcessDirectory(path); err != nil {
			log.Fatal(err)
		}
	default:
		flag.Usage()
	}
}
