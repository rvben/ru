package utils

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI color codes
const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Red        = "\033[31m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Magenta    = "\033[35m"
	Cyan       = "\033[36m"
	White      = "\033[37m"
	BrightBlue = "\033[94m"
	Gray       = "\033[90m"
)

// Log levels
const (
	LevelDebug = iota
	LevelInfo
	LevelWarning
	LevelError
)

var (
	logLevel  = LevelInfo
	useColors = true
)

// SetLogLevel sets the minimum log level to display
func SetLogLevel(level int) {
	logLevel = level
}

// DisableColors turns off color output in logs
func DisableColors() {
	useColors = false
}

// EnableColors turns on color output in logs
func EnableColors() {
	useColors = true
}

// updateLogLevelFromVerbose updates the log level based on the verbose setting
func updateLogLevelFromVerbose() {
	if verbose {
		logLevel = LevelDebug
	} else {
		logLevel = LevelInfo
	}
}

// colorize applies color to text if colors are enabled
func colorize(color string, text string) string {
	if useColors {
		return color + text + Reset
	}
	return text
}

// formatPrefix creates a formatted prefix for log messages
func formatPrefix(level string, category string) string {
	timestamp := time.Now().Format("15:04:05")

	if category == "" {
		return fmt.Sprintf("%s %s",
			colorize(Gray, timestamp),
			colorize(Bold, level))
	}

	return fmt.Sprintf("%s %s [%s]",
		colorize(Gray, timestamp),
		colorize(Bold, level),
		colorize(Cyan, category))
}

// Debug logs a debug message if verbose mode is enabled
func Debug(category string, format string, args ...interface{}) {
	updateLogLevelFromVerbose() // Ensure log level is set correctly
	if verbose && logLevel <= LevelDebug {
		prefix := formatPrefix(colorize(Blue, "DEBUG"), category)
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", prefix, message)
	}
}

// Info logs an informational message if verbose mode is enabled
func Info(category string, format string, args ...interface{}) {
	updateLogLevelFromVerbose() // Ensure log level is set correctly
	if verbose && logLevel <= LevelInfo {
		prefix := formatPrefix(colorize(Green, "INFO "), category)
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", prefix, message)
	}
}

// Warning logs a warning message
func Warning(format string, args ...interface{}) {
	if logLevel <= LevelWarning {
		prefix := formatPrefix(colorize(Yellow, "WARN "), "")
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", prefix, message)
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	if logLevel <= LevelError {
		prefix := formatPrefix(colorize(Red, "ERROR"), "")
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", prefix, message)
	}
}

// Success logs a success message
func Success(format string, args ...interface{}) {
	prefix := formatPrefix(colorize(Green, "OK   "), "")
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s %s\n", prefix, message)
}

// FormatPackageName formats a package name for consistent display
func FormatPackageName(name string) string {
	return colorize(Bold, name)
}

// FormatVersion formats a version string for consistent display
func FormatVersion(version string) string {
	return colorize(Yellow, version)
}

// FormatURL formats a URL for consistent display
func FormatURL(url string) string {
	return colorize(BrightBlue, url)
}

// FormatHTTPStatus formats an HTTP status for consistent display
func FormatHTTPStatus(status string) string {
	if strings.HasPrefix(status, "2") {
		return colorize(Green, status)
	} else if strings.HasPrefix(status, "4") || strings.HasPrefix(status, "5") {
		return colorize(Red, status)
	}
	return colorize(Yellow, status)
}

// Deprecated: Use the new logging functions instead
// VerboseLog is kept for backward compatibility
func VerboseLog(v ...interface{}) {
	if verbose {
		// Convert all arguments to strings
		parts := make([]string, len(v))
		for i, arg := range v {
			parts[i] = fmt.Sprint(arg)
		}

		// Join with spaces and log as debug
		message := strings.Join(parts, " ")
		Debug("legacy", "%s", message)
	}
}
