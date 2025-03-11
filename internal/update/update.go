package update

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/rvben/ru/internal/packagemanager"
	"github.com/rvben/ru/internal/packagemanager/npm"
	"github.com/rvben/ru/internal/packagemanager/pypi"
	"github.com/rvben/ru/internal/packagemanager/pyproject"
	"github.com/rvben/ru/internal/utils"
	ignore "github.com/sabhiram/go-gitignore"
)

type Updater struct {
	pypi           packagemanager.PackageManager
	npm            *npm.NPM
	filesUpdated   int
	filesUnchanged int
	modulesUpdated int
	paths          []string
	verify         bool
	ignorer        *ignore.GitIgnore
	dryRun         bool
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
	// If no paths specified, use current directory
	if len(u.paths) == 0 {
		u.paths = []string{"."}
	}

	// Process each path
	for _, path := range u.paths {
		pathInfo, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("failed to access path %s: %w", path, err)
		}

		// If it's a file, handle it directly
		if !pathInfo.IsDir() {
			utils.VerboseLog("Processing file directly:", path)
			ext := strings.ToLower(filepath.Ext(path))
			filename := filepath.Base(path)

			switch {
			case ext == ".txt" || strings.Contains(strings.ToLower(filename), "requirements"):
				if err := u.updateRequirementsFile(path); err != nil {
					return fmt.Errorf("failed to update requirements file %s: %w", path, err)
				}
			case filename == "package.json":
				if err := u.updatePackageJsonFile(path); err != nil {
					return fmt.Errorf("failed to update package.json file %s: %w", path, err)
				}
			case ext == ".toml" || strings.Contains(strings.ToLower(filename), "pyproject"):
				if err := u.updatePyProjectFile(path); err != nil {
					return fmt.Errorf("failed to update pyproject.toml file %s: %w", path, err)
				}
			default:
				return fmt.Errorf("unsupported file type: %s", path)
			}
			continue
		}

		// Initialize gitignore if needed
		if u.ignorer == nil {
			// Try to load .gitignore file from the path
			gitignorePath := filepath.Join(path, ".gitignore")
			if _, err := os.Stat(gitignorePath); err == nil {
				utils.VerboseLog("Loading .gitignore from", gitignorePath)
				ignorer, err := ignore.CompileIgnoreFile(gitignorePath)
				if err != nil {
					utils.VerboseLog("Error loading .gitignore:", err)
				} else {
					u.ignorer = ignorer
				}
			}
		}

		utils.VerboseLog("Processing directory:", path)
		if err := u.ProcessDirectory(path); err != nil {
			return err
		}
	}

	// Print summary
	if u.filesUpdated > 0 {
		if !u.dryRun {
			utils.Success("%d files updated with %d packages upgraded", u.filesUpdated, u.modulesUpdated)
		} else {
			utils.Info("dry-run", "Would update %d files with %d packages", u.filesUpdated, u.modulesUpdated)
		}
	} else {
		utils.Info("info", "No updates were made. All packages are already at their latest versions.")
	}

	return nil
}

func (u *Updater) ProcessDirectory(dir string) error {
	// Get all relevant files
	requirementsFiles, packageJSONFiles, pyprojectFiles, err := u.findRequirementsFiles(dir)
	if err != nil {
		return err
	}

	// Log counts
	utils.VerboseLog("Found", len(requirementsFiles), "requirements files")
	utils.VerboseLog("Found", len(packageJSONFiles), "package.json files")
	utils.VerboseLog("Found", len(pyprojectFiles), "pyproject.toml files")

	// Process all found files
	var wg sync.WaitGroup
	var errorsMu sync.Mutex
	var errors []error

	// Process the requirements files
	for _, reqFile := range requirementsFiles {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			err := u.updateRequirementsFile(filePath)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, err)
				errorsMu.Unlock()
			}
		}(reqFile)
	}

	// Process package.json files
	for _, packageJSONFile := range packageJSONFiles {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			err := u.updatePackageJsonFile(filePath)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, err)
				errorsMu.Unlock()
			}
		}(packageJSONFile)
	}

	// Process pyproject.toml files
	for _, pyprojectFile := range pyprojectFiles {
		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			err := u.processPyProjectFile(filePath)
			if err != nil {
				errorsMu.Lock()
				errors = append(errors, err)
				errorsMu.Unlock()
			}
		}(pyprojectFile)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Check if there were any errors
	if len(errors) > 0 {
		// Combine all error messages
		var combinedError string
		for _, err := range errors {
			combinedError += err.Error() + "\n"
		}
		return fmt.Errorf("failed to process some files: %s", combinedError)
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
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%s: error reading file: %w", filePath, err)
	}

	// Set custom index URL if specified in requirements.txt
	pypiPkg, ok := u.pypi.(*pypi.PyPI)
	if ok {
		pypiPkg.SetIndexURLFromRequirements(string(content))
	}

	lines := strings.Split(string(content), "\n")
	results := make([]result, 0, len(lines))
	versions := make(map[string]string)

	// First pass: get all package versions
	for i, line := range lines {
		// Parse the line
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			// Skip empty lines and comments
			results = append(results, result{line: line, updatedLine: line, lineNumber: i})
			continue
		}

		// Check for index URL in requirements.txt
		if strings.HasPrefix(strings.TrimSpace(line), "-i ") || strings.HasPrefix(strings.TrimSpace(line), "--index-url ") {
			// This is an index URL line, keep it as is
			results = append(results, result{line: line, updatedLine: line, lineNumber: i})
			continue
		}

		// Check for extra index URL in requirements.txt
		if strings.HasPrefix(strings.TrimSpace(line), "--extra-index-url ") {
			// This is an extra index URL line, keep it as is
			results = append(results, result{line: line, updatedLine: line, lineNumber: i})
			continue
		}

		// Extract package name and version
		packageName, versionConstraints := extractPackageAndVersion(line)
		if packageName == "" {
			// If no package name found, keep the line as is
			results = append(results, result{line: line, updatedLine: line, lineNumber: i})
			continue
		}

		// Get latest version
		latestVersion, err := u.pypi.GetLatestVersion(packageName)
		if err != nil {
			utils.VerboseLog("Error getting latest version for %s: %v", packageName, err)
			// If there's an error, keep the original line
			results = append(results, result{line: line, updatedLine: line, lineNumber: i, packageName: packageName, versionConstraints: versionConstraints})
			continue
		}

		// Store for later verification
		versions[packageName] = latestVersion

		// Check if update is needed and allowed
		if versionConstraints != "" {
			isAllowed, err := u.checkVersionConstraints(latestVersion, versionConstraints)
			if err != nil || !isAllowed {
				// If update is not allowed, keep the original line
				if err != nil {
					utils.VerboseLog("Error checking constraints for %s: %v", packageName, err)
				} else {
					utils.VerboseLog("Update not allowed for %s due to constraints", packageName)
				}
				results = append(results, result{line: line, updatedLine: line, lineNumber: i, packageName: packageName, versionConstraints: versionConstraints})
				continue
			}
		}

		// Process the line for updating
		updatedLine, err := u.updateLine(line, packageName, versionConstraints, latestVersion)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", filePath, i+1, err)
		}

		// Only count as a result if the line was actually changed
		if line != updatedLine {
			results = append(results, result{line: line, updatedLine: updatedLine, lineNumber: i, packageName: packageName, versionConstraints: versionConstraints})
		} else {
			results = append(results, result{line: line, updatedLine: line, lineNumber: i, packageName: packageName, versionConstraints: versionConstraints})
		}
	}

	// Count how many packages actually changed
	changedPackages := 0
	for _, r := range results {
		if r.line != r.updatedLine {
			changedPackages++
		}
	}

	// If we have changes, update the file
	if changedPackages > 0 {
		var updatedContent strings.Builder
		for _, r := range results {
			updatedContent.WriteString(r.updatedLine)
			updatedContent.WriteString("\n")
		}

		// Handle verification if needed
		if u.verify {
			err := u.verifyRequirements(filePath, updatedContent.String())
			if err != nil {
				return fmt.Errorf("%s: verification failed: %w", filePath, err)
			}
		}

		// Write the file or show dry run output
		if !u.dryRun {
			// Write the updated content back to the file
			err = os.WriteFile(filePath, []byte(updatedContent.String()), 0644)
			if err != nil {
				return fmt.Errorf("%s: error writing file: %w", filePath, err)
			}
		} else {
			// In dry run mode, just log what would be done
			utils.Info("dry-run", "Would update file: %s", filePath)
			for _, r := range results {
				if r.line != r.updatedLine && r.packageName != "" {
					utils.Info("dry-run", "  Would update %s: %s -> %s", r.packageName, r.versionConstraints, versions[r.packageName])
				}
			}
		}

		u.filesUpdated++
		u.modulesUpdated += changedPackages
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
	// Read the package.json file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	// Parse the JSON
	var packageJSON map[string]interface{}
	if err := json.Unmarshal(content, &packageJSON); err != nil {
		return fmt.Errorf("error parsing JSON in %s: %w", filePath, err)
	}

	// Check for dependencies
	dependencies, ok := packageJSON["dependencies"].(map[string]interface{})
	if !ok {
		dependencies = make(map[string]interface{})
	}

	// Check for devDependencies
	devDependencies, ok := packageJSON["devDependencies"].(map[string]interface{})
	if !ok {
		devDependencies = make(map[string]interface{})
	}

	// Update dependencies
	updatedDeps := make(map[string]string)
	for name, version := range dependencies {
		versionStr, ok := version.(string)
		if !ok {
			continue
		}

		// Skip git dependencies
		if strings.Contains(versionStr, "git") {
			continue
		}

		// Get the latest version
		latestVersion, err := u.npm.GetLatestVersion(name)
		if err != nil {
			utils.VerboseLog("Error getting latest version for %s: %v", name, err)
			continue
		}

		// Preserve version prefixes (^, ~, etc.)
		prefix := ""
		for _, p := range []string{"^", "~", ">=", "<=", ">", "<"} {
			if strings.HasPrefix(versionStr, p) {
				prefix = p
				break
			}
		}

		updatedVersion := prefix + latestVersion
		if updatedVersion != versionStr {
			updatedDeps[name] = updatedVersion
			dependencies[name] = updatedVersion
		}
	}

	// Update devDependencies
	updatedDevDeps := make(map[string]string)
	for name, version := range devDependencies {
		versionStr, ok := version.(string)
		if !ok {
			continue
		}

		// Skip git dependencies
		if strings.Contains(versionStr, "git") {
			continue
		}

		// Get the latest version
		latestVersion, err := u.npm.GetLatestVersion(name)
		if err != nil {
			utils.VerboseLog("Error getting latest version for %s: %v", name, err)
			continue
		}

		// Preserve version prefixes (^, ~, etc.)
		prefix := ""
		for _, p := range []string{"^", "~", ">=", "<=", ">", "<"} {
			if strings.HasPrefix(versionStr, p) {
				prefix = p
				break
			}
		}

		updatedVersion := prefix + latestVersion
		if updatedVersion != versionStr {
			updatedDevDeps[name] = updatedVersion
			devDependencies[name] = updatedVersion
		}
	}

	// If no dependencies were updated, just return
	if len(updatedDeps) == 0 && len(updatedDevDeps) == 0 {
		u.filesUnchanged++
		return nil
	}

	// Update the package.json
	packageJSON["dependencies"] = dependencies
	packageJSON["devDependencies"] = devDependencies

	// Marshal back to JSON with indentation
	updatedJSON, err := json.MarshalIndent(packageJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling JSON for %s: %w", filePath, err)
	}

	// Write the updated JSON back to file
	if !u.dryRun {
		if err := os.WriteFile(filePath, updatedJSON, 0644); err != nil {
			return fmt.Errorf("error writing to file %s: %w", filePath, err)
		}
	} else {
		// In dry run mode, just log what would be done
		utils.Info("dry-run", "Would update file: %s", filePath)
		for name, version := range updatedDeps {
			utils.Info("dry-run", "  Would update dependency %s -> %s", name, version)
		}
		for name, version := range updatedDevDeps {
			utils.Info("dry-run", "  Would update devDependency %s -> %s", name, version)
		}
	}

	// Update statistics
	u.filesUpdated++
	u.modulesUpdated += len(updatedDeps) + len(updatedDevDeps)

	return nil
}

func (u *Updater) updatePyProjectFile(filePath string) error {
	// Read file content to check for custom index URL
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%s: error reading file: %w", filePath, err)
	}

	// Set custom index URL if specified in the pyproject.toml file
	pypiPkg, ok := u.pypi.(*pypi.PyPI)
	if ok {
		pypiPkg.SetIndexURLFromPyProjectTOML(content)
	}

	// Create or get the PyProject instance
	pyproj := pyproject.NewPyProject(filePath)

	// Get packages that need to be updated
	packageVersionMap := make(map[string]string)

	// Use regex to extract package names and current versions
	// This regex extracts package names from various TOML formats:
	// 1. "package==1.0.0" in arrays
	// 2. package = "^1.0.0" in tables
	re := regexp.MustCompile(`(?m)"([a-zA-Z0-9_.-]+)(?:\[[a-zA-Z0-9_,.-]+\])?(?:==|>=|<=|!=|~=|>|<|===)([^"]+)"`)

	matches := re.FindAllStringSubmatch(string(content), -1)
	utils.VerboseLog("Found", len(matches), "potential packages in the TOML file")

	// Process all matches
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		pkgName := match[1]
		// Clean the package name by removing any quotes or extra whitespace
		pkgName = strings.Trim(pkgName, `"' `)

		if pkgName == "" {
			continue
		}

		// Use the package manager to get the latest version
		latestVersion, err := u.pypi.GetLatestVersion(pkgName)
		if err != nil {
			utils.VerboseLog("Package not found:", pkgName, "(keeping current version)")
			continue
		}

		utils.VerboseLog("Found package", pkgName, "latest version:", latestVersion)
		packageVersionMap[pkgName] = latestVersion
	}

	// Also check for Poetry-style dependencies (package = "version")
	contentStr := string(content)

	// Define valid dependency section headers to check against
	validSections := []string{
		"[tool.poetry.dependencies]",
		"[tool.poetry.dev-dependencies]",
		"[project.dependencies]",
		"[project.optional-dependencies]",
		"[dependency-groups]",
	}

	// Check if a position is within a valid dependency section
	isInValidSection := func(pos int) bool {
		// Find the last section header before this position
		lastSectionPos := -1
		lastSectionHeader := ""

		for _, section := range validSections {
			sectionPos := strings.LastIndex(contentStr[:pos], section)
			if sectionPos > lastSectionPos {
				lastSectionPos = sectionPos
				lastSectionHeader = section
			}
		}

		// No valid section found before this position
		if lastSectionPos == -1 {
			return false
		}

		// Check if there's another section header between the last valid section and this position
		nextSectionPos := findNextSection(contentStr, lastSectionPos+len(lastSectionHeader))
		return nextSectionPos == -1 || nextSectionPos > pos
	}

	// Find all potential Poetry-style dependencies
	rePoetry := regexp.MustCompile(`(?m)([a-zA-Z0-9_.-]+)\s*=\s*["']([^"']+)["']`)
	matchesPoetry := rePoetry.FindAllStringSubmatchIndex(contentStr, -1)

	for _, matchIndices := range matchesPoetry {
		if len(matchIndices) < 4 {
			continue
		}

		// Check if this match is within a valid dependency section
		matchPos := matchIndices[0]
		if !isInValidSection(matchPos) {
			continue
		}

		// Extract the package name using the capture group indices
		pkgNameStart, pkgNameEnd := matchIndices[2], matchIndices[3]
		pkgName := contentStr[pkgNameStart:pkgNameEnd]

		// Clean the package name by removing any quotes or extra whitespace
		pkgName = strings.Trim(pkgName, `"' `)

		if pkgName == "" {
			continue
		}

		// Use the package manager to get the latest version
		latestVersion, err := u.pypi.GetLatestVersion(pkgName)
		if err != nil {
			utils.VerboseLog("Package not found:", pkgName, "(keeping current version)")
			continue
		}

		utils.VerboseLog("Found package", pkgName, "latest version:", latestVersion)
		packageVersionMap[pkgName] = latestVersion
	}

	// Update the pyproject.toml file with the new versions
	updatedModules, err := pyproj.LoadAndUpdate(packageVersionMap)
	if err != nil {
		return fmt.Errorf("failed to update pyproject.toml: %w", err)
	}

	// Update statistics
	if len(updatedModules) > 0 {
		if !u.dryRun {
			// Save changes to the file
			if err := pyproj.Save(); err != nil {
				return fmt.Errorf("failed to save pyproject.toml: %w", err)
			}
		} else {
			// In dry run mode, just log what would be done
			utils.Info("dry-run", "Would update file: %s", filePath)
			for _, pkgName := range updatedModules {
				utils.Info("dry-run", "  Would update %s -> %s", pkgName, packageVersionMap[pkgName])
			}
		}
		u.filesUpdated++
		u.modulesUpdated += len(updatedModules)
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

		// Skip directories that are in .gitignore
		if info.IsDir() {
			if u.ignorer != nil && u.ignorer.MatchesPath(path) {
				utils.VerboseLog("Ignoring directory:", path)
				return filepath.SkipDir
			}
			return nil
		}

		// Check if the file should be ignored
		relPath, err := filepath.Rel(absDir, path)
		if err != nil {
			return err
		}
		if u.ignorer != nil && u.ignorer.MatchesPath(relPath) {
			utils.VerboseLog("Ignoring file:", relPath)
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

// processPyProjectFile processes a pyproject.toml file to update dependencies
func (u *Updater) processPyProjectFile(filePath string) error {
	// Read file content to check for custom index URL
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%s: error reading file: %w", filePath, err)
	}

	// Set custom index URL if specified in the pyproject.toml file
	pypiPkg, ok := u.pypi.(*pypi.PyPI)
	if ok {
		pypiPkg.SetIndexURLFromPyProjectTOML(content)
	}

	// Create or get the PyProject instance
	pyproj := pyproject.NewPyProject(filePath)

	// Get packages that need to be updated
	packageVersionMap := make(map[string]string)

	// Use regex to extract package names and current versions
	// This regex extracts package names from various TOML formats:
	// 1. "package==1.0.0" in arrays
	// 2. package = "^1.0.0" in tables
	re := regexp.MustCompile(`(?m)"([a-zA-Z0-9_.-]+)(?:\[[a-zA-Z0-9_,.-]+\])?(?:==|>=|<=|!=|~=|>|<|===)([^"]+)"`)

	matches := re.FindAllStringSubmatch(string(content), -1)
	utils.VerboseLog("Found", len(matches), "potential packages in the TOML file")

	// Process all matches
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		pkgName := match[1]
		// Clean the package name by removing any quotes or extra whitespace
		pkgName = strings.Trim(pkgName, `"' `)

		if pkgName == "" {
			continue
		}

		// Use the package manager to get the latest version
		latestVersion, err := u.pypi.GetLatestVersion(pkgName)
		if err != nil {
			utils.VerboseLog("Package not found:", pkgName, "(keeping current version)")
			continue
		}

		utils.VerboseLog("Found package", pkgName, "latest version:", latestVersion)
		packageVersionMap[pkgName] = latestVersion
	}

	// Also check for Poetry-style dependencies (package = "version")
	rePoetry := regexp.MustCompile(`(?m)([a-zA-Z0-9_.-]+)\s*=\s*["']([^"']+)["']`)

	// Define valid dependency section headers to check against
	validSections := []string{
		"[tool.poetry.dependencies]",
		"[tool.poetry.dev-dependencies]",
		"[project.dependencies]",
		"[project.optional-dependencies]",
		"[dependency-groups]",
	}

	contentStr := string(content)

	// Check if the match is within a valid dependency section
	isInValidSection := func(pos int) bool {
		// Find the last section header before this position
		lastSectionPos := -1
		lastSectionHeader := ""

		for _, section := range validSections {
			sectionPos := strings.LastIndex(contentStr[:pos], section)
			if sectionPos > lastSectionPos {
				lastSectionPos = sectionPos
				lastSectionHeader = section
			}
		}

		// No valid section found before this position
		if lastSectionPos == -1 {
			return false
		}

		// Check if there's another section header between the last valid section and this position
		nextSectionPos := findNextSection(contentStr, lastSectionPos+len(lastSectionHeader))
		return nextSectionPos == -1 || nextSectionPos > pos
	}

	// Find all potential Poetry-style dependencies
	matchesPoetry := rePoetry.FindAllStringSubmatchIndex(contentStr, -1)

	for _, matchIndices := range matchesPoetry {
		if len(matchIndices) < 4 {
			continue
		}

		// Check if this match is within a valid dependency section
		matchPos := matchIndices[0]
		if !isInValidSection(matchPos) {
			continue
		}

		// Extract the package name using the capture group indices
		pkgNameStart, pkgNameEnd := matchIndices[2], matchIndices[3]
		pkgName := contentStr[pkgNameStart:pkgNameEnd]

		// Clean the package name by removing any quotes or extra whitespace
		pkgName = strings.Trim(pkgName, `"' `)

		if pkgName == "" {
			continue
		}

		// Use the package manager to get the latest version
		latestVersion, err := u.pypi.GetLatestVersion(pkgName)
		if err != nil {
			utils.VerboseLog("Package not found:", pkgName, "(keeping current version)")
			continue
		}

		utils.VerboseLog("Found package", pkgName, "latest version:", latestVersion)
		packageVersionMap[pkgName] = latestVersion
	}

	// Update the pyproject.toml file with the new versions
	updatedModules, err := pyproj.LoadAndUpdate(packageVersionMap)
	if err != nil {
		return fmt.Errorf("failed to update pyproject.toml: %w", err)
	}

	// Update statistics
	u.filesUpdated++
	u.modulesUpdated += len(updatedModules)

	return nil
}

// findNextSection finds the next TOML section header after the given position
func findNextSection(content string, startPos int) int {
	sectionRegex := regexp.MustCompile(`(?m)^\[.*?\]`)
	matches := sectionRegex.FindAllStringIndex(content[startPos:], -1)
	if len(matches) > 0 {
		return startPos + matches[0][0]
	}
	return -1
}

// SetDryRun sets the dry run mode
func (u *Updater) SetDryRun(dryRun bool) {
	u.dryRun = dryRun
}

// extractPackageAndVersion extracts the package name and version constraints from a requirement line
func extractPackageAndVersion(line string) (string, string) {
	line = strings.TrimSpace(line)

	// Skip if line is empty or a comment
	if line == "" || strings.HasPrefix(line, "#") {
		return "", ""
	}

	// Skip if line is a pip option
	if strings.HasPrefix(line, "--") || strings.HasPrefix(line, "-i ") {
		return "", ""
	}

	// Skip git+ lines
	if strings.HasPrefix(line, "git+") {
		return "", ""
	}

	// Match package name and version constraint
	re := regexp.MustCompile(`^([a-zA-Z0-9-_.]+(?:\[[a-zA-Z0-9-_,]+\])?)([<>=!~]+.*)?`)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 2 {
		return "", ""
	}

	packageName := matches[1]
	// Extract base package name without extras
	if idx := strings.Index(packageName, "["); idx != -1 {
		packageName = packageName[:idx]
	}

	versionConstraints := ""
	if len(matches) > 2 && matches[2] != "" {
		versionConstraints = matches[2]
	}

	return packageName, versionConstraints
}
