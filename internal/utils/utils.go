package utils

import (
	"log"
	"net/url"
	"strconv"
)

var verbose bool

// SetVerbose sets the verbose logging flag
func SetVerbose(v bool) {
	verbose = v
}

// VerboseLog prints log messages only if verbose mode is enabled
func VerboseLog(v ...interface{}) {
	if verbose {
		log.Println(v...)
	}
}

func MustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}

func ParseURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}
