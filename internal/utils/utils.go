package utils

import (
	"net/url"
	"strconv"
)

var verbose bool

// SetVerbose sets the verbose logging flag
func SetVerbose(v bool) {
	verbose = v

	// No need to set log level - it's handled internally
}

// IsVerbose returns whether verbose mode is enabled
func IsVerbose() bool {
	return verbose
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
