package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/rvben/ru/internal/packagemanager/pypi"
	"github.com/rvben/ru/internal/update"
	"github.com/rvben/ru/internal/utils"
)

// version is the current version of the tool
const version = "0.1.20"

func main() {
	// CLI Flags
	flag.Usage = func() {
		fmt.Println("Usage:")
		fmt.Println("  ru update [path]   Update requirements*.txt and package.json files in the specified path (default: current directory)")
		fmt.Println("  ru version         Show the version of the tool")
		fmt.Println("  ru help            Show this help message")
		fmt.Println("\nFlags:")
		fmt.Println("  -verbose           Enable verbose logging")
		fmt.Println("  -no-cache          Disable caching")
		fmt.Println("\nExamples:")
		fmt.Println("  ru update          Update requirements*.txt and package.json files in the current directory")
		fmt.Println("  ru update /path/to/dir  Update requirements*.txt and package.json files in the specified directory")
	}
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	noCacheFlag := flag.Bool("no-cache", false, "Disable caching")

	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		flag.Usage()
		return
	}

	utils.SetVerbose(*verboseFlag)

	switch args[0] {
	case "version":
		fmt.Printf("ru %s\n", version)
	case "help":
		flag.Usage()
	case "update":
		path := "."
		if len(args) > 1 {
			path = args[1]
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
