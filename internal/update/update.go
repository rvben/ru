package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	semv "github.com/Masterminds/semver/v3"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sync/errgroup"

	"github.com/rvben/ru/internal/packagemanager/npm"
	"github.com/rvben/ru/internal/packagemanager/pypi"
	"github.com/rvben/ru/internal/packagemanager/pyproject"
	"github.com/rvben/ru/internal/utils"
	ignore "github.com/sabhiram/go-gitignore"
)

type Updater struct {
	pypi           *pypi.PyPI
	npm            *npm.NPM
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
	paths          []string
}

func New(noCache bool, paths []string) *Updater {
	return &Updater{
		pypi:           pypi.New(noCache),
		npm:            npm.New(),
		filesUpdated:   0,
		filesUnchanged: 0,
		modulesUpdated: 0,
		paths:          paths,
	}
}

func (u *Updater) Run() error {
	// If no paths provided, use current directory
	if len(u.paths) == 0 {
		return u.ProcessDirectory(".")
	}

	// Process each provided path
	for _, path := range u.paths {
		utils.VerboseLog("Processing path:", path)
		if err := u.ProcessDirectory(path); err != nil {
			return err
		}
	}

	return nil
}

func (u *Updater) ProcessDirectory(path string) error {
	utils.VerboseLog("Starting to process the directory:", path)

	// If path is a file, process just that file
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		utils.VerboseLog("Processing single file:", path)
		switch {
		case strings.HasSuffix(path, "requirements.txt") ||
			strings.Contains(filepath.Base(path), "requirements"):
			err := u.updateRequirementsFile(path)
			if err != nil {
				return err
			}
			// Print summary for single file
			if u.filesUpdated > 0 {
				fmt.Printf("%d file updated and %d modules updated\n", u.filesUpdated, u.modulesUpdated)
			} else {
				fmt.Printf("%d file left unchanged\n", u.filesUnchanged)
			}
			return nil
		case filepath.Base(path) == "package.json":
			return u.updatePackageJsonFile(path)
		case filepath.Base(path) == "pyproject.toml":
			return u.updatePyProjectFile(path)
		default:
			return fmt.Errorf("unsupported file type: %s", path)
		}
	}

	// Directory processing starts here
	basePath := path
	if basePath == "" {
		basePath = "."
	}

	// Load .gitignore file
	ignoreFile := filepath.Join(basePath, ".gitignore")
	var ignorer *ignore.GitIgnore
	if _, err := os.Stat(ignoreFile); err == nil {
		ignorer, err = ignore.CompileIgnoreFile(ignoreFile)
		if err != nil {
			return fmt.Errorf("error compiling .gitignore file: %w", err)
		}
	}

	foundFiles := 0
	err := filepath.Walk(basePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories that are in .gitignore
		if info.IsDir() {
			if ignorer != nil && ignorer.MatchesPath(filePath) {
				utils.VerboseLog("Ignoring directory:", filePath)
				return filepath.SkipDir
			}
			return nil
		}

		// Check if the file should be ignored
		relPath, err := filepath.Rel(".", filePath)
		if err != nil {
			return err
		}
		if ignorer != nil && ignorer.MatchesPath(relPath) {
			utils.VerboseLog("Ignoring file:", relPath)
			return nil
		}

		switch {
		case strings.HasSuffix(filePath, "requirements.txt") ||
			strings.Contains(filepath.Base(filePath), "requirements"):
			foundFiles++
			return u.updateRequirementsFile(filePath)
		case filepath.Base(filePath) == "package.json":
			foundFiles++
			return u.updatePackageJsonFile(filePath)
		case filepath.Base(filePath) == "pyproject.toml":
			foundFiles++
			return u.updatePyProjectFile(filePath)
		}
		return nil
	})

	if err != nil {
		return err
	}

	if foundFiles == 0 {
		return fmt.Errorf("no supported files found in %s", path)
	}

	if u.filesUpdated > 0 {
		if u.filesUpdated == 1 {
			fmt.Printf("%d file updated and %d modules updated\n", u.filesUpdated, u.modulesUpdated)
		} else {
			fmt.Printf("%d files updated and %d modules updated\n", u.filesUpdated, u.modulesUpdated)
		}
	} else {
		if u.filesUnchanged == 1 {
			fmt.Printf("%d file left unchanged\n", u.filesUnchanged)
		} else {
			fmt.Printf("%d files left unchanged\n", u.filesUnchanged)
		}
	}

	utils.VerboseLog("Completed processing.")
	return nil
}

type result struct {
	line               string
	updatedLine        string
	err                error
	lineNumber         int
	packageName        string
	versionConstraints string
}

func (u *Updater) updateRequirementsFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s:1: error opening file: %w", filePath, err)
	}
	defer file.Close()

	uniqueLines := make(map[string]struct{})
	var sortedLines []string
	modulesUpdatedInFile := 0

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	var g errgroup.Group
	results := make(chan result)

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#") {
				sortedLines = append(sortedLines, line)
			}
			continue
		}

		// Skip lines that start with 'git+'
		if strings.HasPrefix(line, "git+") {
			utils.VerboseLog("Skipping git+ line:", line)
			continue
		}

		// Allow dots in the package name
		re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)([<>=!~]+.*)?`)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			return fmt.Errorf("%s:%d: invalid line format: %s", filePath, lineNumber, line)
		}

		packageName := matches[1]
		versionConstraints := matches[2]
		utils.VerboseLog("Processing package:", packageName)

		g.Go(func() error {
			latestVersion, err := u.pypi.GetLatestVersion(packageName)
			if err != nil {
				return fmt.Errorf("%s:%d: failed to get latest version for package %s: %w", filePath, lineNumber, packageName, err)
			}

			updatedLine, err := u.updateLine(line, packageName, versionConstraints, latestVersion)
			if err != nil {
				return fmt.Errorf("%s:%d: error updating line: %w", filePath, lineNumber, err)
			}

			results <- result{line: line, updatedLine: updatedLine, lineNumber: lineNumber, packageName: packageName, versionConstraints: versionConstraints}
			return nil
		})
	}

	go func() {
		g.Wait()
		close(results)
	}()

	for res := range results {
		if res.err != nil {
			return res.err
		}

		if res.updatedLine != res.line {
			modulesUpdatedInFile++
		}

		uniqueLines[res.updatedLine] = struct{}{}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s:%d: error reading file: %w", filePath, lineNumber, err)
	}

	for line := range uniqueLines {
		sortedLines = append(sortedLines, line)
	}
	sort.Strings(sortedLines)

	output := strings.Join(sortedLines, "\n") + "\n"

	err = os.WriteFile(filePath, []byte(output), 0644)
	if err != nil {
		return fmt.Errorf("%s:1: error writing updated file: %w", filePath, err)
	}

	if modulesUpdatedInFile > 0 {
		u.filesUpdated++
		u.modulesUpdated += modulesUpdatedInFile
	} else {
		u.filesUnchanged++
	}

	return nil
}

func (u *Updater) updateLine(line, packageName, versionConstraints, latestVersion string) (string, error) {
	if versionConstraints != "" {
		if strings.HasPrefix(versionConstraints, "==") {
			if strings.TrimPrefix(versionConstraints, "==") != latestVersion {
				return fmt.Sprintf("%s==%s", packageName, latestVersion), nil
			}
			return line, nil
		}
		ok, err := u.checkVersionConstraints(latestVersion, versionConstraints)
		if err != nil {
			return "", err
		}
		if ok {
			utils.VerboseLog("Latest version is within the specified range:", latestVersion)
			return line, nil
		}
		utils.VerboseLog(fmt.Sprintf("Warning: Latest version %s for package %s is not within the specified range (%s)", latestVersion, packageName, versionConstraints))
		return line, nil
	}
	if !strings.HasSuffix(line, "=="+latestVersion) {
		return fmt.Sprintf("%s==%s", packageName, latestVersion), nil
	}
	return line, nil
}

func (u *Updater) checkVersionConstraints(latestVersion, versionConstraints string) (bool, error) {
	v, err := semv.NewVersion(latestVersion)
	if err != nil {
		return false, fmt.Errorf("error parsing version: %w", err)
	}

	if strings.HasPrefix(versionConstraints, "~=") {
		return u.checkCompatibleRelease(v, versionConstraints[2:])
	} else if strings.HasPrefix(versionConstraints, "==") {
		return latestVersion == strings.TrimPrefix(versionConstraints, "=="), nil
	}

	c, err := semv.NewConstraint(versionConstraints)
	if err != nil {
		return false, fmt.Errorf("error parsing constraint: %w", err)
	}

	return c.Check(v), nil
}

func (u *Updater) checkCompatibleRelease(v *semv.Version, constraint string) (bool, error) {
	parts := strings.Split(constraint, ".")
	var lowerBound, upperBound *semv.Version
	var err error

	if len(parts) == 2 {
		// For ~=X.Y, allow X.Y.0 <= version < (X+1).0.0
		lowerBound, err = semv.NewVersion(constraint + ".0")
		if err != nil {
			return false, fmt.Errorf("error parsing lower bound version: %w", err)
		}
		upperBound, err = semv.NewVersion(fmt.Sprintf("%d.0.0", utils.MustAtoi(parts[0])+1))
		if err != nil {
			return false, fmt.Errorf("error parsing upper bound version: %w", err)
		}
	} else if len(parts) == 3 {
		// For ~=X.Y.Z, allow X.Y.Z <= version < X.(Y+1).0
		lowerBound, err = semv.NewVersion(constraint)
		if err != nil {
			return false, fmt.Errorf("error parsing lower bound version: %w", err)
		}
		upperBound, err = semv.NewVersion(fmt.Sprintf("%s.%d.0", parts[0], utils.MustAtoi(parts[1])+1))
		if err != nil {
			return false, fmt.Errorf("error parsing upper bound version: %w", err)
		}
	} else {
		return false, fmt.Errorf("invalid constraint format: %s", constraint)
	}

	return v.Compare(lowerBound) >= 0 && v.Compare(upperBound) < 0, nil
}

func (u *Updater) updatePackageJsonFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s:1: error opening file: %w", filePath, err)
	}
	defer file.Close()

	// If file is empty, skip with warning
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("%s:1: error getting file info: %w", filePath, err)
	}
	if stat.Size() == 0 {
		utils.VerboseLog("Warning: File is empty:", filePath)
		return nil
	}

	var data map[string]interface{}
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("%s:1: error decoding JSON: %w", filePath, err)
	}

	dependencies, ok := data["dependencies"].(map[string]interface{})
	if !ok {
		utils.VerboseLog("Warning: No dependencies found in package.json:", filePath)
		return nil
	}

	utils.VerboseLog("Found dependencies:", dependencies)

	modulesUpdatedInFile := 0
	npmManager := npm.New() // Use the Npm package manager

	var g errgroup.Group
	var mu sync.Mutex

	for packageName, version := range dependencies {
		packageName := packageName // capture range variable
		version := version         // capture range variable

		g.Go(func() error {
			latestVersion, err := npmManager.GetLatestVersion(packageName)
			if err != nil {
				return fmt.Errorf("%s:1: failed to get latest version for package %s: %w", filePath, packageName, err)
			}

			mu.Lock()
			defer mu.Unlock()
			if version != latestVersion {
				dependencies[packageName] = latestVersion
				modulesUpdatedInFile++
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if modulesUpdatedInFile > 0 {
		u.filesUpdated++
		u.modulesUpdated += modulesUpdatedInFile
	} else {
		u.filesUnchanged++
	}

	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("%s:1: error encoding JSON: %w", filePath, err)
	}

	if err := os.WriteFile(filePath, output, 0644); err != nil {
		return fmt.Errorf("%s:1: error writing updated file: %w", filePath, err)
	}

	return nil
}

func (u *Updater) updatePyProjectFile(filePath string) error {
	utils.VerboseLog("Processing pyproject.toml file:", filePath)

	// Read original content to preserve formatting
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	var proj pyproject.PyProject
	if err := toml.Unmarshal(originalContent, &proj); err != nil {
		return fmt.Errorf("%s: failed to parse pyproject.toml: %w", filePath, err)
	}

	// Collect versions
	versions := make(map[string]string)
	var g errgroup.Group
	var mu sync.Mutex

	for _, dep := range proj.Project.Dependencies {
		parts := strings.Split(dep, "==")
		if len(parts) != 2 {
			continue
		}
		packageName := strings.TrimSpace(parts[0])

		if proj.ShouldIgnorePackage(packageName) {
			continue
		}

		g.Go(func() error {
			latestVersion, err := u.pypi.GetLatestVersion(packageName)
			if err != nil {
				return fmt.Errorf("failed to get latest version for package %s: %w", packageName, err)
			}

			mu.Lock()
			versions[packageName] = latestVersion
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Call the package-level function instead of a method
	if err := pyproject.LoadAndUpdate(filePath, versions); err != nil {
		return fmt.Errorf("%s: %w", filePath, err)
	}

	return nil
}
