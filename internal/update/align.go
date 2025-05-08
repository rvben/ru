package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	semv "github.com/Masterminds/semver/v3"
	"github.com/rvben/ru/internal/utils"
	ignore "github.com/sabhiram/go-gitignore"
)

type Aligner struct {
	pythonVersions map[string]string
	npmVersions    map[string]string
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
	ignorer        *ignore.GitIgnore
}

func NewAligner() *Aligner {
	return &Aligner{
		pythonVersions: make(map[string]string),
		npmVersions:    make(map[string]string),
	}
}

func (a *Aligner) Run() error {
	// Load .gitignore file
	ignoreFile := filepath.Join(".", ".gitignore")
	if _, err := os.Stat(ignoreFile); err == nil {
		a.ignorer, err = ignore.CompileIgnoreFile(ignoreFile)
		if err != nil {
			return fmt.Errorf("error compiling .gitignore file: %w", err)
		}
	}

	// First pass: collect all versions
	if err := a.collectVersions("."); err != nil {
		return err
	}

	// Second pass: align versions
	if err := a.alignVersions("."); err != nil {
		return err
	}

	// Print summary
	if a.filesUpdated > 0 {
		if a.filesUpdated == 1 {
			fmt.Printf("%d file aligned and %d modules updated\n", a.filesUpdated, a.modulesUpdated)
		} else {
			fmt.Printf("%d files aligned and %d modules updated\n", a.filesUpdated, a.modulesUpdated)
		}
	} else {
		if a.filesUnchanged == 1 {
			fmt.Printf("%d file left unchanged\n", a.filesUnchanged)
		} else {
			fmt.Printf("%d files left unchanged\n", a.filesUnchanged)
		}
	}

	return nil
}

func (a *Aligner) collectVersions(path string) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories that are in .gitignore
		if info.IsDir() {
			if a.ignorer != nil && a.ignorer.MatchesPath(filePath) {
				utils.VerboseLog("Ignoring directory:", filePath)
				return filepath.SkipDir
			}
			return nil
		}

		// Check if the file should be ignored
		relPath, err := filepath.Rel(".", filePath)
		if err != nil {
			// If we can't make a relative path, just use the absolute path for ignore checks
			relPath = filePath
		}
		if a.ignorer != nil && a.ignorer.MatchesPath(relPath) {
			utils.VerboseLog("Ignoring file:", relPath)
			return nil
		}

		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			return a.collectPythonVersions(filePath)
		case filepath.Base(filePath) == "package.json":
			return a.collectNPMVersions(filePath)
		}
		return nil
	})
}

func (a *Aligner) collectPythonVersions(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)==([a-zA-Z0-9.*+!~_=<>-]+)`)

	type versionInfo struct {
		highestConcrete string
		highestWildcard string
	}
	versionMap := make(map[string]*versionInfo)

	isWildcard := func(v string) bool {
		return strings.HasSuffix(v, ".*")
	}

	wildcardBase := func(v string) string {
		if idx := strings.LastIndex(v, ".*"); idx != -1 {
			return v[:idx]
		}
		return v
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			pkg, version := matches[1], matches[2]
			if _, ok := versionMap[pkg]; !ok {
				versionMap[pkg] = &versionInfo{}
			}
			vi := versionMap[pkg]
			if isWildcard(version) {
				// Track highest wildcard
				if vi.highestWildcard == "" {
					vi.highestWildcard = version
				} else {
					v1 := wildcardBase(version)
					v2 := wildcardBase(vi.highestWildcard)
					v1Sem, err1 := semv.NewVersion(v1 + ".0")
					v2Sem, err2 := semv.NewVersion(v2 + ".0")
					if err1 == nil && err2 == nil && v1Sem.GreaterThan(v2Sem) {
						vi.highestWildcard = version
					}
				}
			} else {
				// Track highest concrete
				if vi.highestConcrete == "" {
					vi.highestConcrete = version
				} else {
					v1, err1 := semv.NewVersion(version)
					v2, err2 := semv.NewVersion(vi.highestConcrete)
					if err1 == nil && err2 == nil && v1.GreaterThan(v2) {
						vi.highestConcrete = version
					}
				}
			}
		}
	}

	// After collecting, set a.pythonVersions to the highest wildcard if present, else highest concrete
	for pkg, vi := range versionMap {
		if vi.highestWildcard != "" {
			a.pythonVersions[pkg] = vi.highestWildcard
		} else if vi.highestConcrete != "" {
			a.pythonVersions[pkg] = vi.highestConcrete
		}
	}

	fmt.Println("pythonVersions for alignment:", a.pythonVersions)
	return scanner.Err()
}

func (a *Aligner) collectNPMVersions(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}

	for name, version := range pkg.Dependencies {
		// Strip any semver operators
		version = strings.TrimLeft(version, "^~>=<")
		if existingVersion, ok := a.npmVersions[name]; ok {
			// Keep the higher version
			v1, err := semv.NewVersion(version)
			if err != nil {
				utils.VerboseLog("Warning: Invalid version format:", version)
				continue
			}
			v2, err := semv.NewVersion(existingVersion)
			if err != nil {
				utils.VerboseLog("Warning: Invalid version format:", existingVersion)
				continue
			}
			if v1.GreaterThan(v2) {
				a.npmVersions[name] = version
			}
		} else {
			a.npmVersions[name] = version
		}
	}
	return nil
}

func (a *Aligner) alignVersions(path string) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories that are in .gitignore
		if info.IsDir() {
			if a.ignorer != nil && a.ignorer.MatchesPath(filePath) {
				utils.VerboseLog("Ignoring directory:", filePath)
				return filepath.SkipDir
			}
			return nil
		}

		// Check if the file should be ignored
		relPath, err := filepath.Rel(".", filePath)
		if err != nil {
			// If we can't make a relative path, just use the absolute path for ignore checks
			relPath = filePath
		}
		if a.ignorer != nil && a.ignorer.MatchesPath(relPath) {
			utils.VerboseLog("Ignoring file:", relPath)
			return nil
		}

		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			return a.alignPythonFile(filePath)
		case filepath.Base(filePath) == "package.json":
			return a.alignNPMFile(filePath)
		}
		return nil
	})
}

func (a *Aligner) alignPythonFile(filePath string) error {
	fmt.Println("alignPythonFile called for:", filePath)
	utils.VerboseLog("Aligning Python file:", filePath)

	input, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")
	updated := false
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)([<>=!~]+.*)?$`)

	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			pkg := matches[1]
			if version, ok := a.pythonVersions[pkg]; ok {
				newLine := fmt.Sprintf("%s==%s", pkg, version)
				if newLine != line {
					lines[i] = newLine
					updated = true
					a.modulesUpdated++
				}
			}
		}
	}

	if updated {
		a.filesUpdated++
		return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644)
	}
	a.filesUnchanged++
	return nil
}

func (a *Aligner) alignNPMFile(filePath string) error {
	utils.VerboseLog("Aligning NPM file:", filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}

	updated := false
	for name, currentVersion := range pkg.Dependencies {
		if version, ok := a.npmVersions[name]; ok {
			// Preserve the version prefix (^ or ~)
			prefix := ""
			if strings.HasPrefix(currentVersion, "^") {
				prefix = "^"
			} else if strings.HasPrefix(currentVersion, "~") {
				prefix = "~"
			}

			newVersion := prefix + version
			if newVersion != currentVersion {
				pkg.Dependencies[name] = newVersion
				updated = true
				a.modulesUpdated++
			}
		}
	}

	if updated {
		a.filesUpdated++
		output, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filePath, output, 0644)
	}
	a.filesUnchanged++
	return nil
}
