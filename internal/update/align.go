package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rvben/ru/internal/utils"
)

type Aligner struct {
	pythonVersions map[string]string
	npmVersions    map[string]string
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
}

func NewAligner() *Aligner {
	return &Aligner{
		pythonVersions: make(map[string]string),
		npmVersions:    make(map[string]string),
	}
}

func (a *Aligner) Run() error {
	// First pass: collect all versions
	if err := a.collectVersions("."); err != nil {
		return err
	}

	// Second pass: align versions
	return a.alignVersions(".")
}

func (a *Aligner) collectVersions(path string) error {
	return filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
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
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)==([0-9.]+)`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			pkg, version := matches[1], matches[2]
			if existingVersion, ok := a.pythonVersions[pkg]; ok {
				// Keep the higher version
				if compareVersions(version, existingVersion) > 0 {
					a.pythonVersions[pkg] = version
				}
			} else {
				a.pythonVersions[pkg] = version
			}
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
		version = strings.TrimLeft(version, "^~>=<")
		if existingVersion, ok := a.npmVersions[name]; ok {
			// Keep the higher version
			if compareVersions(version, existingVersion) > 0 {
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
		if err != nil || info.IsDir() {
			return err
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
	utils.VerboseLog("Aligning Python file:", filePath)

	input, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")
	updated := false
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)([<>=!~]+.*)`)

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
