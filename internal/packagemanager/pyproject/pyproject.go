package pyproject

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/rvben/ru/internal/utils"
)

type PyProject struct {
	DependencyGroups map[string][]interface{} `toml:"dependency-groups"`
	Project          struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`
	Tool struct {
		Ru struct {
			IgnoreUpdates []string `toml:"ignore-updates"`
		} `toml:"ru"`
	} `toml:"tool"`
}

func (p *PyProject) shouldIgnorePackage(packageName string) bool {
	for _, ignored := range p.Tool.Ru.IgnoreUpdates {
		if ignored == packageName {
			utils.VerboseLog("Ignoring package:", packageName)
			return true
		}
	}
	return false
}

func (p *PyProject) UpdateVersions(versions map[string]string) (bool, error) {
	updated := false

	// Update dependency-groups
	for groupName, deps := range p.DependencyGroups {
		utils.VerboseLog("Processing dependency group:", groupName)
		for i, dep := range deps {
			switch v := dep.(type) {
			case string:
				if pkg := getPackageName(v); p.shouldIgnorePackage(pkg) {
					continue
				}
				if newDep, changed := updateDependencyVersion(v, versions); changed {
					deps[i] = newDep
					updated = true
				}
			case map[string]interface{}:
				// Handle dependency group includes - no version updates needed
				if _, ok := v["include-group"]; !ok {
					return false, fmt.Errorf("invalid dependency object in group %s: %v", groupName, v)
				}
			default:
				return false, fmt.Errorf("invalid dependency type in group %s: %T", groupName, dep)
			}
		}
	}

	// Update project dependencies
	for i, dep := range p.Project.Dependencies {
		if pkg := getPackageName(dep); p.shouldIgnorePackage(pkg) {
			continue
		}
		if newDep, changed := updateDependencyVersion(dep, versions); changed {
			p.Project.Dependencies[i] = newDep
			updated = true
		}
	}

	// Update optional dependencies
	for _, deps := range p.Project.OptionalDependencies {
		for i, dep := range deps {
			if pkg := getPackageName(dep); p.shouldIgnorePackage(pkg) {
				continue
			}
			if newDep, changed := updateDependencyVersion(dep, versions); changed {
				deps[i] = newDep
				updated = true
			}
		}
	}

	return updated, nil
}

func updateDependencyVersion(dep string, versions map[string]string) (string, bool) {
	// Skip empty lines and comments
	if dep == "" || strings.HasPrefix(dep, "#") {
		return dep, false
	}

	// Parse the dependency string
	parts := strings.Split(dep, "==")
	if len(parts) != 2 {
		return dep, false
	}

	packageName := strings.TrimSpace(parts[0])
	if newVersion, ok := versions[packageName]; ok {
		newDep := fmt.Sprintf("%s==%s", packageName, newVersion)
		if newDep != dep {
			utils.VerboseLog("Updating", packageName, "from", parts[1], "to", newVersion)
			return newDep, true
		}
	}

	return dep, false
}

// Helper function to extract package name from dependency string
func getPackageName(dep string) string {
	// Skip empty lines and comments
	if dep == "" || strings.HasPrefix(dep, "#") {
		return ""
	}

	// Parse the dependency string
	parts := strings.Split(dep, "==")
	if len(parts) < 1 {
		return ""
	}

	return strings.TrimSpace(parts[0])
}

func LoadAndUpdate(filename string, versions map[string]string) error {
	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read pyproject.toml: %w", err)
	}

	// Parse TOML
	var pyproject PyProject
	if err := toml.Unmarshal(data, &pyproject); err != nil {
		return fmt.Errorf("failed to parse pyproject.toml: %w", err)
	}

	// Update versions
	updated, err := pyproject.UpdateVersions(versions)
	if err != nil {
		return fmt.Errorf("failed to update versions: %w", err)
	}

	if !updated {
		return nil
	}

	// Marshal back to TOML
	newData, err := toml.Marshal(pyproject)
	if err != nil {
		return fmt.Errorf("failed to marshal updated pyproject.toml: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, newData, 0644); err != nil {
		return fmt.Errorf("failed to write updated pyproject.toml: %w", err)
	}

	return nil
}
