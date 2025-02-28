package utils

import (
	"fmt"
	"strings"
	"sync"
)

// Version represents a parsed semantic version with optimized storage and comparison
// Inspired by astral-sh/uv PR #789 (https://github.com/astral-sh/uv/pull/789)
type Version struct {
	Raw          string // Original version string
	Parts        []int  // Parsed version parts (major.minor.patch)
	Suffix       string // Pre-release/build suffix
	IsPreRelease bool   // Whether this is a pre-release version
	IsValid      bool   // Whether the version is valid
	Error        error  // Error if the version is invalid
}

// versionCache is a simple in-memory cache for parsed versions
var versionCache = make(map[string]*Version)
var versionCacheMutex sync.RWMutex

// ParseVersion parses a version string into a Version object
// This function is now optimized with a cache to avoid repeatedly parsing the same version strings
func ParseVersion(version string) *Version {
	// Quick check for empty version
	if version == "" {
		return &Version{
			Raw:     "",
			Parts:   []int{0, 0, 0},
			IsValid: false,
			Error:   fmt.Errorf("empty version string"),
		}
	}

	// Check cache first (read lock)
	versionCacheMutex.RLock()
	if v, ok := versionCache[version]; ok {
		versionCacheMutex.RUnlock()
		return v
	}
	versionCacheMutex.RUnlock()

	// Parse version and store in cache (write lock)
	v := parseVersionInternal(version)

	// Only cache valid versions to avoid filling cache with invalid data
	if v.IsValid {
		versionCacheMutex.Lock()
		// Double-check in case another goroutine has added it while we were parsing
		if _, ok := versionCache[version]; !ok {
			versionCache[version] = v
		}
		versionCacheMutex.Unlock()
	}

	return v
}

// parseVersionInternal parses a version string without using the cache
func parseVersionInternal(version string) *Version {
	// Check for empty or obviously invalid versions
	if version == "" {
		return &Version{
			Raw:     version,
			IsValid: false,
			Error:   fmt.Errorf("empty version string"),
		}
	}

	// Split version into parts and suffix
	var suffix string
	verStr := version

	// Handle pre-release suffixes like -alpha, -beta, etc.
	isPreRelease := false
	for _, sep := range []string{"-", "+", "a", "b", "rc"} {
		if idx := strings.Index(verStr, sep); idx >= 0 {
			suffix = verStr[idx:]
			verStr = verStr[:idx]
			isPreRelease = true
			break
		}
	}

	// Split into numeric parts
	parts := strings.Split(verStr, ".")
	numParts := make([]int, 0, 3)

	// Ensure the base version has at least one numeric part
	if len(parts) == 0 || parts[0] == "" {
		return &Version{
			Raw:     version,
			IsValid: false,
			Error:   fmt.Errorf("invalid version format: %s", version),
		}
	}

	// Check if the parts are valid numbers
	valid := true
	for _, part := range parts {
		if part == "" {
			valid = false
			break
		}

		num := 0
		parsed := false
		for _, c := range part {
			if c >= '0' && c <= '9' {
				num = num*10 + int(c-'0')
				parsed = true
			} else {
				valid = false
				break
			}
		}

		if !valid || !parsed {
			break
		}

		numParts = append(numParts, num)
	}

	if !valid || len(numParts) == 0 {
		return &Version{
			Raw:     version,
			IsValid: false,
			Error:   fmt.Errorf("invalid version format: %s", version),
		}
	}

	// Ensure we have at least 3 parts (major.minor.patch)
	for len(numParts) < 3 {
		numParts = append(numParts, 0)
	}

	return &Version{
		Raw:          version,
		Parts:        numParts,
		Suffix:       suffix,
		IsPreRelease: isPreRelease,
		IsValid:      true,
	}
}

// Compare compares two versions
// Returns:
//
//	-1 if v < other
//	 0 if v == other
//	 1 if v > other
func (v *Version) Compare(other *Version) int {
	// If either version is invalid, they can't be compared
	if !v.IsValid || !other.IsValid {
		return 0
	}

	// Fast path: if raw strings are equal, versions are equal
	if v.Raw == other.Raw {
		return 0
	}

	// Compare numeric parts
	for i := 0; i < 3; i++ {
		if i >= len(v.Parts) {
			if i >= len(other.Parts) {
				break
			}
			return -1 // v has fewer parts
		}
		if i >= len(other.Parts) {
			return 1 // other has fewer parts
		}

		if v.Parts[i] < other.Parts[i] {
			return -1
		}
		if v.Parts[i] > other.Parts[i] {
			return 1
		}
	}

	// If numeric parts are equal, compare pre-release status
	if v.IsPreRelease && !other.IsPreRelease {
		return -1 // Pre-release is less than stable
	}
	if !v.IsPreRelease && other.IsPreRelease {
		return 1 // Stable is greater than pre-release
	}

	// If both are pre-release or both are stable, lexicographically compare suffixes
	if v.Suffix < other.Suffix {
		return -1
	}
	if v.Suffix > other.Suffix {
		return 1
	}

	return 0 // Versions are equal
}

// IsGreaterThan returns true if v > other
func (v *Version) IsGreaterThan(other *Version) bool {
	// If either version is invalid, return false
	if !v.IsValid || !other.IsValid {
		return false
	}
	return v.Compare(other) > 0
}

// IsLessThan returns true if v < other
func (v *Version) IsLessThan(other *Version) bool {
	// If either version is invalid, return false
	if !v.IsValid || !other.IsValid {
		return false
	}
	return v.Compare(other) < 0
}

// IsEqual returns true if v == other
func (v *Version) IsEqual(other *Version) bool {
	// If either version is invalid, return false
	if !v.IsValid || !other.IsValid {
		return false
	}
	return v.Compare(other) == 0
}

// IsCompatible checks if the version is compatible with the given constraint
// It handles various constraint operators (==, >=, <=, ~=, etc.)
func (v *Version) IsCompatible(constraint string) bool {
	// If this version is invalid, it can't be compatible
	if !v.IsValid {
		return false
	}

	// Fast path for exact match or equality
	if constraint == v.Raw || constraint == "=="+v.Raw {
		return true
	}

	// Handle different constraint operators
	if strings.HasPrefix(constraint, "==") {
		other := ParseVersion(strings.TrimPrefix(constraint, "=="))
		return other.IsValid && v.IsEqual(other)
	}
	if strings.HasPrefix(constraint, ">=") {
		other := ParseVersion(strings.TrimPrefix(constraint, ">="))
		return other.IsValid && (v.IsGreaterThan(other) || v.IsEqual(other))
	}
	if strings.HasPrefix(constraint, "<=") {
		other := ParseVersion(strings.TrimPrefix(constraint, "<="))
		return other.IsValid && (v.IsLessThan(other) || v.IsEqual(other))
	}
	if strings.HasPrefix(constraint, ">") {
		other := ParseVersion(strings.TrimPrefix(constraint, ">"))
		return other.IsValid && v.IsGreaterThan(other)
	}
	if strings.HasPrefix(constraint, "<") {
		other := ParseVersion(strings.TrimPrefix(constraint, "<"))
		return other.IsValid && v.IsLessThan(other)
	}
	if strings.HasPrefix(constraint, "~=") {
		// Compatible release: same major version, minor version greater or equal
		other := ParseVersion(strings.TrimPrefix(constraint, "~="))
		return other.IsValid && v.Parts[0] == other.Parts[0] && (v.Parts[1] > other.Parts[1] ||
			(v.Parts[1] == other.Parts[1] && v.Parts[2] >= other.Parts[2]))
	}
	if strings.HasPrefix(constraint, "^") {
		// Caret constraint: allow changes that do not modify the left-most non-zero digit
		other := ParseVersion(strings.TrimPrefix(constraint, "^"))
		if !other.IsValid {
			return false
		}
		if other.Parts[0] > 0 {
			return v.Parts[0] == other.Parts[0] && v.Compare(other) >= 0
		} else if other.Parts[1] > 0 {
			return v.Parts[0] == 0 && v.Parts[1] == other.Parts[1] && v.Compare(other) >= 0
		} else {
			return v.Parts[0] == 0 && v.Parts[1] == 0 && v.Parts[2] == other.Parts[2] && v.Compare(other) >= 0
		}
	}

	// No constraint operator, compare directly
	other := ParseVersion(constraint)
	return other.IsValid && v.IsEqual(other)
}

// ClearVersionCache clears the version cache
func ClearVersionCache() {
	versionCacheMutex.Lock()
	versionCache = make(map[string]*Version)
	versionCacheMutex.Unlock()
}
