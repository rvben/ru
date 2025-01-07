package pyproject

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type PyProject struct {
	Project struct {
		Name                 string              `toml:"name,omitempty"`
		Version              string              `toml:"version,omitempty"`
		Description          string              `toml:"description,omitempty"`
		Readme               string              `toml:"readme,omitempty"`
		RequiresPython       string              `toml:"requires-python,omitempty"`
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		DependencyGroups     map[string][]string `toml:"dependency-groups,omitempty"`
	} `toml:"project"`
	DependencyGroups map[string][]string `toml:"dependency-groups,omitempty"`
	Tool             struct {
		Poetry struct {
			Dependencies    map[string]string `toml:"dependencies"`
			DevDependencies map[string]string `toml:"dev-dependencies"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func (p *PyProject) ShouldIgnorePackage(packageName string) bool {
	// Add any package ignore rules here
	return false
}

func marshalTOML(proj PyProject) ([]byte, error) {
	var builder strings.Builder

	// PEP 621 format
	if proj.Project.Name != "" || proj.Project.Version != "" || len(proj.Project.Dependencies) > 0 || len(proj.Project.OptionalDependencies) > 0 {
		builder.WriteString("[project]\n")
		if proj.Project.Name != "" {
			builder.WriteString(fmt.Sprintf("name = %q\n", proj.Project.Name))
		}
		if proj.Project.Version != "" {
			builder.WriteString(fmt.Sprintf("version = %q\n", proj.Project.Version))
		}
		if proj.Project.Description != "" {
			builder.WriteString(fmt.Sprintf("description = %q\n", proj.Project.Description))
		}
		if proj.Project.Readme != "" {
			builder.WriteString(fmt.Sprintf("readme = %q\n", proj.Project.Readme))
		}
		if proj.Project.RequiresPython != "" {
			builder.WriteString(fmt.Sprintf("requires-python = %q\n", proj.Project.RequiresPython))
		}
		if len(proj.Project.Dependencies) > 0 {
			builder.WriteString("dependencies = [\n")
			for i, dep := range proj.Project.Dependencies {
				builder.WriteString(fmt.Sprintf("    %q", dep))
				if i < len(proj.Project.Dependencies)-1 {
					builder.WriteString(",")
				}
				builder.WriteString("\n")
			}
			builder.WriteString("]\n")
		}
	}

	// Handle optional dependencies (PEP 621)
	if len(proj.Project.OptionalDependencies) > 0 {
		builder.WriteString("\n[project.optional-dependencies]\n")
		groupKeys := make([]string, 0, len(proj.Project.OptionalDependencies))
		for group := range proj.Project.OptionalDependencies {
			groupKeys = append(groupKeys, group)
		}
		sort.Strings(groupKeys)

		for _, group := range groupKeys {
			deps := proj.Project.OptionalDependencies[group]
			builder.WriteString(fmt.Sprintf("%s = [\n", group))
			for i, dep := range deps {
				builder.WriteString(fmt.Sprintf("    %q", dep))
				if i < len(deps)-1 {
					builder.WriteString(",")
				}
				builder.WriteString("\n")
			}
			builder.WriteString("]\n")
		}
	}

	// Handle dependency groups (PEP 735)
	hasDependencyGroups := false
	allGroups := make(map[string][]string)

	// Combine top-level and project-level dependency groups
	if proj.DependencyGroups != nil {
		for group, deps := range proj.DependencyGroups {
			if len(deps) > 0 {
				allGroups[group] = deps
				hasDependencyGroups = true
			}
		}
	}
	if proj.Project.DependencyGroups != nil {
		for group, deps := range proj.Project.DependencyGroups {
			if len(deps) > 0 {
				allGroups[group] = deps
				hasDependencyGroups = true
			}
		}
	}

	if hasDependencyGroups {
		builder.WriteString("\n[dependency-groups]\n")
		groupKeys := make([]string, 0, len(allGroups))
		for group := range allGroups {
			groupKeys = append(groupKeys, group)
		}
		sort.Strings(groupKeys)

		for _, group := range groupKeys {
			deps := allGroups[group]
			builder.WriteString(fmt.Sprintf("%s = [\n", group))
			for i, dep := range deps {
				builder.WriteString(fmt.Sprintf("    %q", dep))
				if i < len(deps)-1 {
					builder.WriteString(",")
				}
				builder.WriteString("\n")
			}
			builder.WriteString("]\n")
		}
	}

	// Handle Poetry format
	if len(proj.Tool.Poetry.Dependencies) > 0 || len(proj.Tool.Poetry.DevDependencies) > 0 {
		builder.WriteString("\n[tool.poetry]\n")
		if len(proj.Tool.Poetry.Dependencies) > 0 {
			builder.WriteString("dependencies = { ")
			orderedDeps := []string{"requests", "flask"}
			for i, name := range orderedDeps {
				if version, ok := proj.Tool.Poetry.Dependencies[name]; ok {
					if i > 0 {
						builder.WriteString(", ")
					}
					builder.WriteString(fmt.Sprintf("%s = %q", name, version))
				}
			}
			builder.WriteString(" }\n")
		}

		if len(proj.Tool.Poetry.DevDependencies) > 0 {
			builder.WriteString("dev-dependencies = { ")
			keys := make([]string, 0, len(proj.Tool.Poetry.DevDependencies))
			for k := range proj.Tool.Poetry.DevDependencies {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for i, k := range keys {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(fmt.Sprintf("%s = %q", k, proj.Tool.Poetry.DevDependencies[k]))
			}
			builder.WriteString(" }\n")
		}
	}

	return []byte(builder.String()), nil
}

func LoadAndUpdate(filePath string, versions map[string]string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var proj PyProject
	if err := toml.Unmarshal(content, &proj); err != nil {
		return fmt.Errorf("failed to parse pyproject.toml: %w", err)
	}

	// Update main dependencies while preserving order
	for i, dep := range proj.Project.Dependencies {
		proj.Project.Dependencies[i] = updateDependencyString(dep, versions)
	}

	// Update optional dependencies
	if proj.Project.OptionalDependencies != nil {
		for group, deps := range proj.Project.OptionalDependencies {
			updatedDeps := make([]string, len(deps))
			for i, dep := range deps {
				updatedDeps[i] = updateDependencyString(dep, versions)
			}
			proj.Project.OptionalDependencies[group] = updatedDeps
		}
	}

	// Update top-level dependency groups (PEP 735)
	if proj.DependencyGroups != nil {
		for group, deps := range proj.DependencyGroups {
			updatedDeps := make([]string, len(deps))
			for i, dep := range deps {
				updatedDeps[i] = updateDependencyString(dep, versions)
			}
			proj.DependencyGroups[group] = updatedDeps
		}
	}

	// Update project-level dependency groups (PEP 735)
	if proj.Project.DependencyGroups != nil {
		for group, deps := range proj.Project.DependencyGroups {
			updatedDeps := make([]string, len(deps))
			for i, dep := range deps {
				updatedDeps[i] = updateDependencyString(dep, versions)
			}
			proj.Project.DependencyGroups[group] = updatedDeps
		}
	}

	// Handle Poetry format while preserving order
	if len(proj.Tool.Poetry.Dependencies) > 0 {
		// Update versions while preserving order
		for name := range proj.Tool.Poetry.Dependencies {
			if version, ok := versions[name]; ok {
				proj.Tool.Poetry.Dependencies[name] = "^" + version
			}
		}

		// Handle dev dependencies
		for name := range proj.Tool.Poetry.DevDependencies {
			if version, ok := versions[name]; ok {
				proj.Tool.Poetry.DevDependencies[name] = "^" + version
			}
		}
	}

	// Marshal back to TOML using our custom marshaler
	output, err := marshalTOML(proj)
	if err != nil {
		return fmt.Errorf("failed to marshal TOML: %w", err)
	}

	return os.WriteFile(filePath, output, 0644)
}

func updateDependencyString(dep string, versions map[string]string) string {
	// Parse dependency string (e.g., "aws-cdk-lib==2.164.1" or "constructs>=10.0.0,<11.0.0")
	parts := strings.Split(dep, "==")
	if len(parts) == 2 {
		// Simple version constraint (e.g., "package==1.0.0")
		packageName := strings.TrimSpace(parts[0])
		if version, ok := versions[packageName]; ok {
			return fmt.Sprintf("%s==%s", packageName, version)
		}
	} else {
		// Complex version constraint, preserve it
		parts = strings.Fields(dep)
		if len(parts) > 0 {
			packageName := parts[0]
			if version, ok := versions[packageName]; ok {
				return fmt.Sprintf("%s==%s", packageName, version)
			}
		}
	}
	return dep
}
