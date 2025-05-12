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
		utils.VerboseLog("Files to process:")
		for _, f := range a.filesToProcess {
			utils.VerboseLog("  ", f)
		}
	}

	// Second: collect all versions from these files
	if err := a.collectVersionsFromFiles(); err != nil {
		return err
	}

	// Verbose: print the highest versions found
	if utils.IsVerbose() {
		if len(a.pythonVersions) > 0 {
			utils.VerboseLog("Highest Python package versions found:")
			for pkg, ver := range a.pythonVersions {
				utils.VerboseLog("  ", pkg, "->", ver)
			}
		}
		if len(a.npmVersions) > 0 {
			utils.VerboseLog("Highest NPM package versions found:")
			for pkg, ver := range a.npmVersions {
				utils.VerboseLog("  ", pkg, "->", ver)
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
				utils.VerboseLog("Ignoring directory:", filePath)
				return filepath.SkipDir
			}
			return nil
		}
		relPath, err := filepath.Rel(".", filePath)
		if err != nil {
			relPath = filePath
		}
		if a.ignorer != nil && a.ignorer.MatchesPath(relPath) {
			utils.VerboseLog("Ignoring file:", relPath)
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
	for _, filePath := range a.filesToProcess {
		switch {
		case strings.HasSuffix(filePath, "requirements.txt"):
			if err := a.collectPythonVersions(filePath); err != nil {
				return err
			}
		case filepath.Base(filePath) == "package.json":
			if err := a.collectNPMVersions(filePath); err != nil {
				return err
			}
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

	// After collecting, set a.pythonVersions to the highest concrete if present, else highest wildcard
	for pkg, vi := range versionMap {
		if vi.highestConcrete != "" {
			a.pythonVersions[pkg] = vi.highestConcrete
		} else if vi.highestWildcard != "" {
			a.pythonVersions[pkg] = vi.highestWildcard
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

func (a *Aligner) alignPythonFile(filePath string) error {
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
