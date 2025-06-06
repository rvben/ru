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
	"github.com/rvben/pyver"
	"github.com/rvben/ru/internal/utils"
	ignore "github.com/sabhiram/go-gitignore"
)

type versionInfo struct {
	highestConcrete string
	highestWildcard string
}

type Aligner struct {
	pythonVersions map[string]string
	npmVersions    map[string]string
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
	ignorer        *ignore.GitIgnore
	filesToProcess []string
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

	// First: collect the list of files to process
	if err := a.collectFilesToProcess("."); err != nil {
		return err
	}

	// Verbose: print the list of files to process
	if utils.IsVerbose() {
		utils.Debug("align", "Files to process:")
		for _, f := range a.filesToProcess {
			utils.Debug("align", "  %s", f)
		}
	}

	// Second: collect all versions from these files
	if err := a.collectVersionsFromFiles(); err != nil {
		return err
	}

	// Verbose: print the highest versions found
	if utils.IsVerbose() {
		if len(a.pythonVersions) > 0 {
			utils.Debug("align", "Highest Python package versions found:")
			for pkg, ver := range a.pythonVersions {
				utils.Debug("align", "  %s -> %s", pkg, ver)
			}
		}
		if len(a.npmVersions) > 0 {
			utils.Debug("align", "Highest NPM package versions found:")
			for pkg, ver := range a.npmVersions {
				utils.Debug("align", "  %s -> %s", pkg, ver)
			}
		}
	}

	// Third: align versions in these files
	if err := a.alignVersionsInFiles(); err != nil {
		return err
	}

	// Print summary
	if a.filesUpdated > 0 {
		if a.filesUpdated == 1 {
			utils.Success("%d file aligned and %d modules updated", a.filesUpdated, a.modulesUpdated)
		} else {
			utils.Success("%d files aligned and %d modules updated", a.filesUpdated, a.modulesUpdated)
		}
	} else {
		if a.filesUnchanged == 1 {
			utils.Info("align", "%d file left unchanged", a.filesUnchanged)
		} else {
			utils.Info("align", "%d files left unchanged", a.filesUnchanged)
		}
	}

	return nil
}

func (a *Aligner) collectFilesToProcess(path string) error {
	a.filesToProcess = nil
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if a.ignorer != nil && a.ignorer.MatchesPath(filePath) {
				utils.Debug("align", "Ignoring directory: %s", filePath)
				return filepath.SkipDir
			}
			return nil
		}
		relPath, err := filepath.Rel(".", filePath)
		if err != nil {
			relPath = filePath
		}
		if a.ignorer != nil && a.ignorer.MatchesPath(relPath) {
			utils.Debug("align", "Ignoring file: %s", relPath)
			return nil
		}
		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			a.filesToProcess = append(a.filesToProcess, filePath)
		case filepath.Base(filePath) == "package.json":
			a.filesToProcess = append(a.filesToProcess, filePath)
		}
		return nil
	})
}

func (a *Aligner) collectVersionsFromFiles() error {
	// Global version map for all Python packages
	versionMap := make(map[string]*versionInfo)

	// First pass: scan all files and aggregate highest versions globally
	for _, filePath := range a.filesToProcess {
		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			if err := a.scanPythonVersions(filePath, versionMap); err != nil {
				return err
			}
		case filepath.Base(filePath) == "package.json":
			if err := a.collectNPMVersions(filePath); err != nil {
				return err
			}
		}
	}

	// After collecting, set a.pythonVersions to the true highest version (concrete only)
	for pkg, vi := range versionMap {
		if vi.highestConcrete != "" {
			a.pythonVersions[pkg] = vi.highestConcrete
		}
	}

	return nil
}

func (a *Aligner) scanPythonVersions(filePath string, versionMap map[string]*versionInfo) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)==([a-zA-Z0-9.*+!~_=<>-]+)`)

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
					utils.Debug("align", "[align] Set initial highest wildcard for %s: %s", pkg, version)
				} else {
					v1 := wildcardBase(version)
					v2 := wildcardBase(vi.highestWildcard)
					v1Ver, err1 := pyver.Parse(v1 + ".0")
					v2Ver, err2 := pyver.Parse(v2 + ".0")
					utils.Debug("align", "[align] Comparing wildcards for %s: %s vs %s", pkg, v1, v2)
					if err1 != nil || err2 != nil {
						utils.Debug("align", "[align] Could not parse wildcard version(s): %s %s err1: %v err2: %v", v1, v2, err1, err2)
					}
					if err1 == nil && err2 == nil && pyver.Compare(v1Ver, v2Ver) > 0 {
						utils.Debug("align", "[align] Updating highest wildcard for %s to %s", pkg, version)
						vi.highestWildcard = version
					}
				}
			} else {
				// Track highest concrete
				v1, err1 := pyver.Parse(version)
				if err1 != nil {
					utils.Warning("Skipping unparsable version for %s: %s", pkg, version)
					continue
				}
				if vi.highestConcrete == "" {
					vi.highestConcrete = version
					utils.Debug("align", "[align] Set initial highest concrete for %s: %s", pkg, version)
				} else {
					v2, err2 := pyver.Parse(vi.highestConcrete)
					if err2 != nil {
						utils.Warning("Skipping unparsable highestConcrete for %s: %s", pkg, vi.highestConcrete)
						vi.highestConcrete = version
						continue
					}
					utils.Debug("align", "[align] Comparing concretes for %s: %s vs %s", pkg, version, vi.highestConcrete)
					if pyver.Compare(v1, v2) > 0 {
						utils.Debug("align", "[align] Updating highest concrete for %s to %s", pkg, version)
						vi.highestConcrete = version
					}
				}
			}
		} else {
			utils.Debug("align", "[align] Skipped line (no match): %s", line)
		}
	}

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
		cleanVersion := strings.TrimLeft(version, "^~>=<")
		if existingVersion, ok := a.npmVersions[name]; ok {
			v1, err1 := semv.NewVersion(cleanVersion)
			v2, err2 := semv.NewVersion(existingVersion)
			if err1 == nil && err2 == nil && v1.GreaterThan(v2) {
				a.npmVersions[name] = cleanVersion
			}
		} else {
			a.npmVersions[name] = cleanVersion
		}
	}
	return nil
}

func (a *Aligner) alignVersionsInFiles() error {
	for _, filePath := range a.filesToProcess {
		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			if err := a.alignPythonFile(filePath); err != nil {
				return err
			}
		case filepath.Base(filePath) == "package.json":
			if err := a.alignNPMFile(filePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Aligner) alignPythonFile(filePath string) error {
	utils.Debug("align", "Aligning Python file: %s", filePath)

	input, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")
	updated := false
	// Match any line that starts with a package name (alphanumeric, dash, underscore, dot), possibly followed by any constraint
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)([<>=!~].*)?$`)

	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			pkg := matches[1]
			if version, ok := a.pythonVersions[pkg]; ok {
				lines[i] = fmt.Sprintf("%s==%s", pkg, version)
				updated = true
				a.modulesUpdated++
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
	utils.Debug("align", "Aligning NPM file: %s", filePath)

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
	for name, version := range pkg.Dependencies {
		// Strip any semver operators
		cleanVersion := strings.TrimLeft(version, "^~>=<")
		if existingVersion, ok := a.npmVersions[name]; ok {
			v1, err1 := semv.NewVersion(cleanVersion)
			v2, err2 := semv.NewVersion(existingVersion)
			if err1 == nil && err2 == nil && v1.GreaterThan(v2) {
				a.npmVersions[name] = cleanVersion
			}
		} else {
			a.npmVersions[name] = cleanVersion
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
