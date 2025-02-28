package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	semv "github.com/Masterminds/semver/v3"
	"golang.org/x/sync/errgroup"

	"github.com/rvben/ru/internal/depgraph"
	"github.com/rvben/ru/internal/packagemanager"
	"github.com/rvben/ru/internal/packagemanager/npm"
	"github.com/rvben/ru/internal/packagemanager/pypi"
	"github.com/rvben/ru/internal/packagemanager/pyproject"
	"github.com/rvben/ru/internal/utils"
)

type Updater struct {
	pypi           packagemanager.PackageManager
	npm            *npm.NPM
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
	paths          []string
	verify         bool
}

func New(noCache bool, verify bool, paths []string) *Updater {
	return &Updater{
		pypi:           pypi.New(noCache),
		npm:            npm.New(),
		filesUpdated:   0,
		filesUnchanged: 0,
		modulesUpdated: 0,
		paths:          paths,
		verify:         verify,
	}
}

func (u *Updater) Run() error {
	// First check if PyPI endpoint is accessible
	if err := u.pypi.SetCustomIndexURL(); err != nil {
		return fmt.Errorf("failed to set custom PyPI index: %w", err)
	}

	// If no paths provided, use current directory
	if len(u.paths) == 0 {
		err := u.ProcessDirectory(".")
		if err != nil {
			return err
		}
	} else {
		// Process each provided path
		for _, path := range u.paths {
			utils.VerboseLog("Processing path:", path)
			if err := u.ProcessDirectory(path); err != nil {
				return err
			}
		}
	}

	// Print summary of updates
	if u.filesUpdated > 0 {
		if u.filesUpdated == 1 {
			if u.modulesUpdated == 1 {
				fmt.Printf("%d file updated with %d package upgraded\n", u.filesUpdated, u.modulesUpdated)
			} else {
				fmt.Printf("%d file updated with %d packages upgraded\n", u.filesUpdated, u.modulesUpdated)
			}
		} else {
			if u.modulesUpdated == 1 {
				fmt.Printf("%d files updated with %d package upgraded\n", u.filesUpdated, u.modulesUpdated)
			} else {
				fmt.Printf("%d files updated with %d packages upgraded\n", u.filesUpdated, u.modulesUpdated)
			}
		}
	} else {
		fmt.Println("No updates were made. All packages are already at their latest versions.")
	}

	return nil
}

func (u *Updater) ProcessDirectory(dir string) error {
	// Convert dir to absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	utils.VerboseLog("Processing directory:", absDir)

	// Get all files in the directory
	files, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(absDir, file.Name())
		switch {
		case strings.HasSuffix(file.Name(), ".txt") && strings.Contains(strings.ToLower(file.Name()), "requirements"):
			utils.VerboseLog("Found requirements file:", filePath)
			if err := u.updateRequirementsFile(filePath); err != nil {
				return fmt.Errorf("failed to update requirements file: %v", err)
			}
		case file.Name() == "pyproject.toml":
			utils.VerboseLog("Found pyproject.toml file:", filePath)
			if err := u.updatePyProjectFile(filePath); err != nil {
				return fmt.Errorf("failed to update pyproject.toml: %v", err)
			}
		case file.Name() == "package.json":
			utils.VerboseLog("Found package.json file:", filePath)
			if err := u.updatePackageJsonFile(filePath); err != nil {
				return fmt.Errorf("failed to update package.json: %v", err)
			}
		}
	}

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
	utils.VerboseLog("Updating requirements file:", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s:1: error opening file: %w", filePath, err)
	}
	defer file.Close()

	// Read all lines first to avoid writing an empty file if there's an error
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: error reading file: %w", filePath, err)
	}

	// Create dependency graph
	graph := depgraph.New()
	packageVersions := make(map[string]string)
	packageConstraints := make(map[string]string)
	currentVersions := make(map[string]string)
	originalLines := make(map[string]string)

	// First pass: collect dependencies and build graph
	lineNumber := 0
	for _, line := range lines {
		lineNumber++
		line := strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip lines that start with 'git+'
		if strings.HasPrefix(line, "git+") {
			utils.VerboseLog("Skipping git+ line:", line)
			continue
		}

		// Allow dots in the package name
		re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+(?:\[[a-zA-Z0-9-_,]+\])?)([<>=!~]+.*)?`)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			return fmt.Errorf("%s:%d: invalid line format: %s", filePath, lineNumber, line)
		}

		packageName := matches[1]
		// Extract base package name without extras for version lookup
		basePackageName := packageName
		if idx := strings.Index(packageName, "["); idx != -1 {
			basePackageName = packageName[:idx]
		}
		versionConstraints := matches[2]

		// Store the original line and constraints
		originalLines[basePackageName] = line
		if versionConstraints != "" {
			packageConstraints[basePackageName] = versionConstraints
			if strings.HasPrefix(versionConstraints, "==") {
				currentVersions[basePackageName] = strings.TrimPrefix(versionConstraints, "==")
			}
		}

		// Add to graph
		graph.AddNode(basePackageName, "")

		// Parse dependencies if this is a package with version constraints
		if versionConstraints != "" {
			// Add dependency relationship
			if err := graph.AddDependency("root", basePackageName, versionConstraints); err != nil {
				return fmt.Errorf("%s:%d: error adding dependency: %w", filePath, lineNumber, err)
			}
		}
	}

	// Detect cycles
	if cycles := graph.DetectCycles(); len(cycles) > 0 {
		fmt.Printf("Warning: Circular dependencies detected in %s:\n", filePath)
		for _, cycle := range cycles {
			fmt.Printf("  %s\n", strings.Join(cycle, " -> "))
		}
	}

	// Get update order
	updateOrder := graph.GetUpdateOrder()

	// Second pass: fetch latest versions and validate updates
	var g errgroup.Group
	results := make(chan struct {
		name    string
		version string
		err     error
	})

	for _, pkg := range updateOrder {
		pkg := pkg // Capture loop variable
		g.Go(func() error {
			latestVersion, err := u.pypi.GetLatestVersion(pkg)
			if err != nil {
				results <- struct {
					name    string
					version string
					err     error
				}{pkg, "", err}
				return nil
			}

			// Validate the update against constraints
			if err := graph.ValidateUpdate(pkg, latestVersion); err != nil {
				utils.VerboseLog(fmt.Sprintf("Warning: Skipping update of %s: %v", pkg, err))
				results <- struct {
					name    string
					version string
					err     error
				}{pkg, "", nil}
				return nil
			}

			results <- struct {
				name    string
				version string
				err     error
			}{pkg, latestVersion, nil}
			return nil
		})
	}

	go func() {
		g.Wait()
		close(results)
	}()

	// Collect results
	modulesUpdatedInFile := 0
	for result := range results {
		if result.err != nil {
			// Print warning but don't fail
			fmt.Printf("Warning: Package not found: %s (keeping current version)\n", result.name)
			continue
		}
		if result.version != "" {
			packageVersions[result.name] = result.version
		}
	}

	// Update the lines with new versions
	var outputLines []string
	updatedPackages := make(map[string]bool)

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			outputLines = append(outputLines, line)
			continue
		}

		if strings.HasPrefix(trimmedLine, "git+") {
			outputLines = append(outputLines, line)
			continue
		}

		re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+(?:\[[a-zA-Z0-9-_,]+\])?)([<>=!~]+.*)?`)
		matches := re.FindStringSubmatch(trimmedLine)
		if len(matches) < 2 {
			outputLines = append(outputLines, line)
			continue
		}

		packageName := matches[1]
		basePackageName := packageName
		if idx := strings.Index(packageName, "["); idx != -1 {
			basePackageName = packageName[:idx]
		}

		if newVersion, ok := packageVersions[basePackageName]; ok {
			var newLine string
			if existingConstraint, hasConstraint := packageConstraints[basePackageName]; hasConstraint {
				// Check if the constraint is a simple equality
				if strings.HasPrefix(existingConstraint, "==") {
					newLine = fmt.Sprintf("%s==%s", packageName, newVersion)
				} else {
					// Preserve the existing constraint
					newLine = fmt.Sprintf("%s%s", packageName, existingConstraint)
				}
			} else {
				// No existing constraint, use exact version
				newLine = fmt.Sprintf("%s==%s", packageName, newVersion)
			}

			// Check if the line actually changed
			if newLine != originalLines[basePackageName] {
				updatedPackages[basePackageName] = true
			}
			outputLines = append(outputLines, newLine)
		} else {
			outputLines = append(outputLines, line)
		}
	}

	// Count actually updated modules
	modulesUpdatedInFile = len(updatedPackages)

	// Sort non-comment lines
	var comments []string
	var packages []string
	for _, line := range outputLines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			comments = append(comments, line)
		} else if strings.TrimSpace(line) != "" {
			packages = append(packages, line)
		}
	}
	sort.Strings(packages)

	// Combine comments and sorted packages
	outputLines = append(comments, packages...)

	if u.verify {
		// Verify dependencies before writing the file
		if err := u.verifyRequirements(filePath, strings.Join(outputLines, "\n")+"\n"); err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
	}

	if err := os.WriteFile(filePath, []byte(strings.Join(outputLines, "\n")+"\n"), 0644); err != nil {
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
			currentVersion := strings.TrimPrefix(versionConstraints, "==")
			if currentVersion != latestVersion {
				return fmt.Sprintf("%s==%s", packageName, latestVersion), nil
			}
			return line, nil
		}

		ok, err := u.checkVersionConstraints(latestVersion, versionConstraints)
		if err != nil {
			return "", err
		}
		if !ok {
			// If version doesn't match constraints, keep the original line
			utils.VerboseLog(fmt.Sprintf("Warning: Latest version %s for package %s is not within the specified range (%s)", latestVersion, packageName, versionConstraints))
			return line, nil
		}
		// If version matches constraints, keep using the constraints
		utils.VerboseLog("Latest version is within the specified range:", latestVersion)
		return line, nil
	}
	// No version constraints, always update to latest
	return fmt.Sprintf("%s==%s", packageName, latestVersion), nil
}

func (u *Updater) checkVersionConstraints(latestVersion, versionConstraints string) (bool, error) {
	v, err := semv.NewVersion(latestVersion)
	if err != nil {
		return false, fmt.Errorf("error parsing version: %w", err)
	}

	if strings.HasPrefix(versionConstraints, "~=") {
		return u.checkCompatibleRelease(v, versionConstraints[2:])
	} else if strings.HasPrefix(versionConstraints, "==") {
		currentVersion := strings.TrimPrefix(versionConstraints, "==")
		cv, err := semv.NewVersion(currentVersion)
		if err != nil {
			return false, fmt.Errorf("error parsing current version: %w", err)
		}
		return v.GreaterThan(cv), nil
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
	utils.VerboseLog("Updating package.json file:", filePath)
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

	// Sort dependencies alphabetically
	sortedDeps := make(map[string]interface{})
	keys := make([]string, 0, len(dependencies))
	for k := range dependencies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sortedDeps[k] = dependencies[k]
	}
	data["dependencies"] = sortedDeps

	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("%s:1: error encoding JSON: %w", filePath, err)
	}

	// Ensure final newline
	output = append(output, '\n')

	if err := os.WriteFile(filePath, output, 0644); err != nil {
		return fmt.Errorf("%s:1: error writing updated file: %w", filePath, err)
	}

	return nil
}

func (u *Updater) updatePyProjectFile(filePath string) error {
	utils.VerboseLog("Updating pyproject.toml file:", filePath)
	// Convert to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Create a new PyProject instance
	proj := pyproject.NewPyProject(absPath)

	// Get latest versions for dependencies
	versions := make(map[string]string)
	if err := u.getLatestVersions(filePath, versions); err != nil {
		return fmt.Errorf("failed to get latest versions: %v", err)
	}

	// Update the file
	updatedModules, err := proj.LoadAndUpdate(versions)
	if err != nil {
		return fmt.Errorf("failed to update pyproject.toml: %v", err)
	}

	if len(updatedModules) > 0 {
		u.filesUpdated++
	} else {
		u.filesUnchanged++
	}

	return nil
}

func (u *Updater) verifyRequirements(filePath string, updatedContent string) error {
	utils.VerboseLog("Verifying dependencies for:", filePath)

	// Check if uv is available
	_, err := exec.LookPath("uv")
	useUV := err == nil
	utils.VerboseLog("Using", map[bool]string{true: "uv", false: "pip"}[useUV], "for dependency verification")

	// Create a temporary directory for venv
	tmpDir, err := os.MkdirTemp("", "ru-verify-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create temporary requirements file
	tmpReq := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(tmpReq, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write temporary requirements: %w", err)
	}

	verifyErr := func() error {
		if useUV {
			// Create virtual environment for uv
			venvPath := filepath.Join(tmpDir, "venv")
			cmd := exec.Command("python", "-m", "venv", venvPath)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to create virtual environment: %w", err)
			}

			// Use uv for dependency checking
			uvCmd := exec.Command(
				"uv",
				"pip",
				"install",
				"--dry-run",
				"--no-deps", // First check without dependencies
				"-r",
				tmpReq,
			)
			uvCmd.Env = append(os.Environ(), fmt.Sprintf("VIRTUAL_ENV=%s", venvPath))
			uvCmd.Env = append(uvCmd.Env, fmt.Sprintf("PATH=%s:%s", filepath.Join(venvPath, "bin"), os.Getenv("PATH")))
			if output, err := uvCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("dependency resolution failed.\nDetails:\n%s", output)
			}

			// Now check with dependencies
			uvCmd = exec.Command(
				"uv",
				"pip",
				"install",
				"--dry-run",
				"-r",
				tmpReq,
			)
			uvCmd.Env = append(os.Environ(), fmt.Sprintf("VIRTUAL_ENV=%s", venvPath))
			uvCmd.Env = append(uvCmd.Env, fmt.Sprintf("PATH=%s:%s", filepath.Join(venvPath, "bin"), os.Getenv("PATH")))
			if output, err := uvCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("dependency resolution failed.\nDetails:\n%s", output)
			}
		} else {
			// Create virtual environment
			cmd := exec.Command("python", "-m", "venv", filepath.Join(tmpDir, "venv"))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to create virtual environment: %w", err)
			}

			// Run pip check in dry-run mode
			pipCmd := exec.Command(
				filepath.Join(tmpDir, "venv", "bin", "pip"),
				"install",
				"--dry-run",
				"--no-deps", // First check without dependencies
				"-r",
				tmpReq,
			)
			if output, err := pipCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("dependency resolution failed.\nDetails:\n%s", output)
			}

			// Now check with dependencies
			pipCmd = exec.Command(
				filepath.Join(tmpDir, "venv", "bin", "pip"),
				"install",
				"--dry-run",
				"-r",
				tmpReq,
			)
			if output, err := pipCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("dependency resolution failed.\nDetails:\n%s", output)
			}
		}
		return nil
	}()

	if verifyErr != nil {
		// Print a more user-friendly message
		fmt.Printf("\nWarning: %s may have compatibility issues:\n\n", filePath)
		fmt.Printf("%s\n\n", verifyErr)
		fmt.Printf("Options:\n")
		fmt.Printf("1. Run with --skip-verify to update anyway\n")
		fmt.Printf("2. Manually review and adjust version constraints\n")
		fmt.Printf("3. Keep the current versions\n\n")
		return fmt.Errorf("dependency verification failed for %s", filePath)
	}

	return nil
}

func (u *Updater) getLatestVersions(filePath string, versions map[string]string) error {
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// Extract package names from the content
	re := regexp.MustCompile(`"([a-zA-Z0-9-_.]+(?:\[[a-zA-Z0-9-_,]+\])?)==[^"]*"`)
	matches := re.FindAllStringSubmatch(string(content), -1)
	for _, match := range matches {
		if len(match) > 1 {
			pkg := match[1]
			if version, err := u.pypi.GetLatestVersion(pkg); err == nil {
				versions[pkg] = version
			} else {
				fmt.Printf("Warning: Package not found: %s (keeping current version)\n", pkg)
			}
		}
	}

	return nil
}
