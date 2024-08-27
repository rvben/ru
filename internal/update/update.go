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

	semv "github.com/Masterminds/semver/v3"

	"github.com/rvben/ru/internal/packagemanager"
	"github.com/rvben/ru/internal/packagemanager/npm"
	"github.com/rvben/ru/internal/utils"
)

type Updater struct {
	pm             packagemanager.PackageManager
	filesUpdated   int
	modulesUpdated int
	filesUnchanged int
}

func NewUpdater(pm packagemanager.PackageManager) *Updater {
	return &Updater{pm: pm}
}

func (u *Updater) ProcessDirectory(path string) error {
	utils.VerboseLog("Starting to process the directory:", path)

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched, err := filepath.Match("requirements*.txt", filepath.Base(filePath))
		if err != nil {
			return err
		}
		if matched {
			utils.VerboseLog("Found:", filePath)
			if err := u.updateRequirementsFile(filePath); err != nil {
				return err
			}
		}

		// Add this block to handle package.json files
		if filepath.Base(filePath) == "package.json" {
			utils.VerboseLog("Found:", filePath)
			if err := u.updatePackageJsonFile(filePath); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	if u.filesUpdated > 0 {
		fmt.Printf("%d file updated and %d modules updated\n", u.filesUpdated, u.modulesUpdated)
	} else {
		fmt.Printf("%d files left unchanged\n", u.filesUnchanged)
	}

	utils.VerboseLog("Completed processing.")
	return nil
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
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#") {
				sortedLines = append(sortedLines, line)
			}
			continue
		}

		re := regexp.MustCompile(`^([a-zA-Z0-9-_]+)([<>=!~]+.*)?`)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			return fmt.Errorf("%s:%d: invalid line format: %s", filePath, lineNumber, line)
		}

		packageName := matches[1]
		versionConstraints := matches[2]
		utils.VerboseLog("Processing package:", packageName)

		latestVersion, err := u.pm.GetLatestVersion(packageName)
		if err != nil {
			return fmt.Errorf("%s:%d: failed to get latest version for package %s: %w", filePath, lineNumber, packageName, err)
		}

		updatedLine, err := u.updateLine(line, packageName, versionConstraints, latestVersion)
		if err != nil {
			return fmt.Errorf("%s:%d: error updating line: %w", filePath, lineNumber, err)
		}

		if updatedLine != line {
			modulesUpdatedInFile++
		}

		uniqueLines[updatedLine] = struct{}{}
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

	var data map[string]interface{}
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("%s:1: error decoding JSON: %w", filePath, err)
	}

	utils.VerboseLog("Parsed JSON data:", data)

	dependencies, ok := data["dependencies"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s:1: no dependencies found in package.json", filePath)
	}

	utils.VerboseLog("Found dependencies:", dependencies)

	modulesUpdatedInFile := 0
	npmManager := npm.New() // Use the Npm package manager

	for packageName, version := range dependencies {
		latestVersion, err := npmManager.GetLatestVersion(packageName)
		if err != nil {
			return fmt.Errorf("%s:1: failed to get latest version for package %s: %w", filePath, packageName, err)
		}

		if version != latestVersion {
			dependencies[packageName] = latestVersion
			modulesUpdatedInFile++
		}
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
