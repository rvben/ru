package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

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

		// Print HTTP metrics if verbose mode is enabled
		if utils.IsVerbose() {
			// Initialize metrics
			var pypiMetrics, npmMetrics utils.HTTPClientMetrics

			// Try to get PyPI metrics using type assertion
			if pypiWithMetrics, ok := u.pypi.(interface {
				GetRequestMetrics() utils.HTTPClientMetrics
			}); ok {
				pypiMetrics = pypiWithMetrics.GetRequestMetrics()
			}

			// Get NPM metrics directly
			if u.npm != nil {
				npmMetrics = u.npm.GetRequestMetrics()
			}

			// Calculate total metrics
			totalRequests := pypiMetrics.RequestCount + npmMetrics.RequestCount
			totalRetries := pypiMetrics.RetryCount + npmMetrics.RetryCount
			totalFailures := pypiMetrics.FailureCount + npmMetrics.FailureCount
			totalSuccesses := pypiMetrics.SuccessCount + npmMetrics.SuccessCount
			totalCircuitBreaks := pypiMetrics.CircuitBreakerTrips + npmMetrics.CircuitBreakerTrips

			// Only show metrics if there were any requests
			if totalRequests > 0 {
				// Calculate average request time
				var avgTimeMs float64
				totalTimeNs := pypiMetrics.TotalRequestTime + npmMetrics.TotalRequestTime
				avgTimeMs = float64(totalTimeNs) / float64(totalRequests) / 1e6 // convert to ms

				fmt.Println("\nHTTP Metrics:")
				fmt.Printf("  Total Requests: %d (PyPI: %d, NPM: %d)\n",
					totalRequests, pypiMetrics.RequestCount, npmMetrics.RequestCount)
				fmt.Printf("  Successful: %d, Failed: %d, Retries: %d\n",
					totalSuccesses, totalFailures, totalRetries)
				if totalCircuitBreaks > 0 {
					fmt.Printf("  Circuit Breaker Trips: %d\n", totalCircuitBreaks)
				}
				fmt.Printf("  Average Request Time: %.2f ms\n", avgTimeMs)
			}
		}
	} else {
		fmt.Println("No updates were made. All packages are already at their latest versions.")
	}

	return nil
}

func (u *Updater) ProcessDirectory(dir string) error {
	// Find all requirements files in the directory
	requirementsFiles, packageJSONFiles, pyprojectFiles, err := u.findRequirementsFiles(dir)
	if err != nil {
		return err
	}

	utils.VerboseLog("Found", len(requirementsFiles), "requirements files")
	utils.VerboseLog("Found", len(packageJSONFiles), "package.json files")
	utils.VerboseLog("Found", len(pyprojectFiles), "pyproject.toml files")

	// Use parallel processing if we have multiple files
	if len(requirementsFiles)+len(packageJSONFiles)+len(pyprojectFiles) > 1 {
		return u.processFilesParallel(requirementsFiles, packageJSONFiles, pyprojectFiles)
	}

	// Process each requirements file
	for _, file := range requirementsFiles {
		if err := u.updateRequirementsFile(file); err != nil {
			return err
		}
	}

	// Process each package.json file
	for _, file := range packageJSONFiles {
		if err := u.updatePackageJsonFile(file); err != nil {
			return err
		}
	}

	// Process each pyproject.toml file
	for _, file := range pyprojectFiles {
		if err := u.updatePyProjectFile(file); err != nil {
			return err
		}
	}

	return nil
}

// processFilesParallel processes multiple files in parallel using a worker pool
func (u *Updater) processFilesParallel(requirementsFiles, packageJSONFiles, pyprojectFiles []string) error {
	type fileJob struct {
		path     string
		fileType string // "requirements", "package.json", or "pyproject.toml"
	}

	// Create a channel for jobs
	jobs := make(chan fileJob, len(requirementsFiles)+len(packageJSONFiles)+len(pyprojectFiles))

	// Create a channel for results
	results := make(chan error, len(requirementsFiles)+len(packageJSONFiles)+len(pyprojectFiles))

	// Determine the number of workers - use min(numCPU*2, numFiles) to avoid creating too many goroutines
	// for small numbers of files
	numFiles := len(requirementsFiles) + len(packageJSONFiles) + len(pyprojectFiles)
	numWorkers := runtime.NumCPU() * 2
	if numWorkers > numFiles {
		numWorkers = numFiles
	}
	utils.VerboseLog("Using", numWorkers, "workers for parallel processing")

	// Start workers
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for job := range jobs {
				var err error
				switch job.fileType {
				case "requirements":
					err = u.updateRequirementsFile(job.path)
				case "package.json":
					err = u.updatePackageJsonFile(job.path)
				case "pyproject.toml":
					err = u.updatePyProjectFile(job.path)
				}
				results <- err
			}
		}()
	}

	// Submit jobs
	for _, file := range requirementsFiles {
		jobs <- fileJob{path: file, fileType: "requirements"}
	}
	for _, file := range packageJSONFiles {
		jobs <- fileJob{path: file, fileType: "package.json"}
	}
	for _, file := range pyprojectFiles {
		jobs <- fileJob{path: file, fileType: "pyproject.toml"}
	}
	close(jobs)

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Check for errors
	for err := range results {
		if err != nil {
			return err
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

func (u *Updater) checkVersionConstraints(latestVerStr, versionConstraints string) (bool, error) {
	// Fast path for simple equality constraints
	if strings.HasPrefix(versionConstraints, "==") && versionConstraints[2:] == latestVerStr {
		return false, nil
	}

	// Parse the latest version
	latestVer := utils.ParseVersion(latestVerStr)
	if !latestVer.IsValid {
		return false, fmt.Errorf("invalid latest version: %s", latestVerStr)
	}

	shouldUpdate := false

	// Check for complex constraints with commas (e.g. ">=1.0.0,<2.0.0")
	if strings.Contains(versionConstraints, ",") {
		parts := strings.Split(versionConstraints, ",")

		// Check if all parts of the constraint are satisfied
		validForAll := true
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !latestVer.IsCompatible(part) {
				validForAll = false
				break
			}
		}

		shouldUpdate = validForAll
	} else if strings.HasPrefix(versionConstraints, "==") {
		// Handle equality constraint
		currentVersion := strings.TrimPrefix(versionConstraints, "==")
		currentVer := utils.ParseVersion(currentVersion)

		// Check if the current version is valid
		if !currentVer.IsValid {
			return false, currentVer.Error
		}

		shouldUpdate = latestVer.IsGreaterThan(currentVer)
	} else if strings.HasPrefix(versionConstraints, ">=") ||
		strings.HasPrefix(versionConstraints, ">") ||
		strings.HasPrefix(versionConstraints, "<=") ||
		strings.HasPrefix(versionConstraints, "<") ||
		strings.HasPrefix(versionConstraints, "~=") ||
		strings.HasPrefix(versionConstraints, "^") {
		// Handle other constraints
		shouldUpdate = latestVer.IsCompatible(versionConstraints)
	} else {
		// Default to simple version comparison
		currentVer := utils.ParseVersion(versionConstraints)

		// Check if the current version is valid
		if !currentVer.IsValid {
			return false, currentVer.Error
		}

		shouldUpdate = latestVer.IsGreaterThan(currentVer)
	}

	return shouldUpdate, nil
}

func (u *Updater) checkCompatibleRelease(v *utils.Version, constraint string) (bool, error) {
	// ~= operator (compatible release)
	specifierVersion := strings.TrimPrefix(constraint, "~=")

	// Parse the specifier version
	specVer := utils.ParseVersion(specifierVersion)

	// Compatible release must have:
	// 1. Same major version
	// 2. Either higher minor version, or same minor and higher or equal patch
	return v.Parts[0] == specVer.Parts[0] &&
		(v.Parts[1] > specVer.Parts[1] ||
			(v.Parts[1] == specVer.Parts[1] && v.Parts[2] >= specVer.Parts[2])), nil
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

	// Simple line-by-line parsing instead of relying on complex regex
	lines := strings.Split(string(content), "\n")
	packageCount := 0

	for _, line := range lines {
		// Skip empty lines and comments
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip git+ lines
		if strings.HasPrefix(line, "git+") {
			continue
		}

		// Basic parsing: split on first occurrence of version specifier
		var packageName string
		for _, prefix := range []string{"==", ">=", "<=", "!=", "~=", ">", "<", "==="} {
			if idx := strings.Index(line, prefix); idx > 0 {
				packageName = strings.TrimSpace(line[:idx])
				utils.VerboseLog("Found package:", packageName, "with version specifier:", prefix)
				packageCount++
				break
			}
		}

		if packageName != "" {
			// Extract base package name without extras
			basePackageName := packageName
			if idx := strings.Index(packageName, "["); idx != -1 {
				basePackageName = packageName[:idx]
			}

			utils.VerboseLog("Getting latest version for package:", basePackageName)
			if version, err := u.pypi.GetLatestVersion(basePackageName); err == nil {
				versions[basePackageName] = version
				utils.VerboseLog("Found latest version for", basePackageName, ":", version)
			} else {
				utils.VerboseLog("Error getting latest version for", basePackageName, ":", err)
				fmt.Printf("Warning: Package not found: %s (keeping current version)\n", basePackageName)
			}
		}
	}

	utils.VerboseLog("Found", packageCount, "packages in", filePath)
	return nil
}

// findRequirementsFiles finds all requirements files, package.json files, and pyproject.toml files in the given directory
func (u *Updater) findRequirementsFiles(dir string) (requirements, packageJSON, pyproject []string, err error) {
	// Convert dir to absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	utils.VerboseLog("Searching for dependency files in:", absDir)

	// Common requirements patterns
	reqPatterns := []string{
		"requirements.txt",
		"requirements-*.txt",
		"requirements_*.txt",
		"*.requirements.txt",
		"requirements-dev.txt",
		"requirements_dev.txt",
	}

	// Initialize return slices
	requirements = []string{}
	packageJSON = []string{}
	pyproject = []string{}

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if the file is a requirements file
		filename := info.Name()
		for _, pattern := range reqPatterns {
			matched, err := filepath.Match(pattern, filename)
			if err != nil {
				return err
			}
			if matched {
				utils.VerboseLog("Found requirements file:", path)
				requirements = append(requirements, path)
				break
			}
		}

		// Check if the file is a package.json file
		if filename == "package.json" {
			utils.VerboseLog("Found package.json file:", path)
			packageJSON = append(packageJSON, path)
		}

		// Check if the file is a pyproject.toml file
		if filename == "pyproject.toml" {
			utils.VerboseLog("Found pyproject.toml file:", path)
			pyproject = append(pyproject, path)
		}

		return nil
	})

	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to walk directory: %v", err)
	}

	return requirements, packageJSON, pyproject, nil
}
