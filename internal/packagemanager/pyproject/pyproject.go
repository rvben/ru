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
			// Sort dependencies
			deps := make([]string, len(proj.Project.Dependencies))
			copy(deps, proj.Project.Dependencies)
			sort.Strings(deps)
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
			// Sort dependencies within the group
			sortedDeps := make([]string, len(deps))
			copy(sortedDeps, deps)
			sort.Strings(sortedDeps)
			for i, dep := range sortedDeps {
				builder.WriteString(fmt.Sprintf("    %q", dep))
				if i < len(sortedDeps)-1 {
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
			// Sort dependencies within the group
			sortedDeps := make([]string, len(deps))
			copy(sortedDeps, deps)
			sort.Strings(sortedDeps)
			for i, dep := range sortedDeps {
				builder.WriteString(fmt.Sprintf("    %q", dep))
				if i < len(sortedDeps)-1 {
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
			// Sort dependencies
			keys := make([]string, 0, len(proj.Tool.Poetry.Dependencies))
			for k := range proj.Tool.Poetry.Dependencies {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for i, k := range keys {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(fmt.Sprintf("%s = %q", k, proj.Tool.Poetry.Dependencies[k]))
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

	// First, unmarshal into our PyProject struct for dependency handling
	var proj PyProject
	if err := toml.Unmarshal(content, &proj); err != nil {
		return fmt.Errorf("failed to parse pyproject.toml: %w", err)
	}

	// Track if any dependencies were updated
	hasChanges := false

	// Split the content into lines for manual updating
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var output []string
	inDependencies := false
	inOptionalDependencies := false
	inDependencyGroups := false
	inPoetryDependencies := false
	skipUntilNextSection := false
	currentSection := ""
	seenSections := make(map[string]bool)
	sectionContent := make(map[string][]string)

	// Update dependencies
	if proj.Project.Dependencies != nil {
		updatedDeps := make([]string, len(proj.Project.Dependencies))
		for i, dep := range proj.Project.Dependencies {
			updatedDep := updateDependencyString(dep, versions)
			if updatedDep != dep {
				hasChanges = true
			}
			updatedDeps[i] = updatedDep
		}
		sort.Strings(updatedDeps)
		proj.Project.Dependencies = updatedDeps
	}

	// Update optional dependencies
	if proj.Project.OptionalDependencies != nil {
		optDeps := make(map[string][]string)
		for group, deps := range proj.Project.OptionalDependencies {
			updatedDeps := make([]string, len(deps))
			for i, dep := range deps {
				updatedDep := updateDependencyString(dep, versions)
				if updatedDep != dep {
					hasChanges = true
				}
				updatedDeps[i] = updatedDep
			}
			sort.Strings(updatedDeps)
			optDeps[group] = updatedDeps
		}
		proj.Project.OptionalDependencies = optDeps
	}

	// Update dependency groups
	if proj.DependencyGroups != nil {
		depGroups := make(map[string][]string)
		for group, deps := range proj.DependencyGroups {
			updatedDeps := make([]string, len(deps))
			for i, dep := range deps {
				updatedDep := updateDependencyString(dep, versions)
				if updatedDep != dep {
					hasChanges = true
				}
				updatedDeps[i] = updatedDep
			}
			sort.Strings(updatedDeps)
			depGroups[group] = updatedDeps
		}
		proj.DependencyGroups = depGroups
	}

	// Update Poetry dependencies
	if proj.Tool.Poetry.Dependencies != nil {
		deps := make(map[string]string)
		keys := make([]string, 0, len(proj.Tool.Poetry.Dependencies))
		for name := range proj.Tool.Poetry.Dependencies {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			if newVersion, ok := versions[name]; ok {
				deps[name] = "^" + newVersion
				hasChanges = true
			} else {
				deps[name] = proj.Tool.Poetry.Dependencies[name]
			}
		}
		proj.Tool.Poetry.Dependencies = deps

		// Handle dev dependencies
		if len(proj.Tool.Poetry.DevDependencies) > 0 {
			devDeps := make(map[string]string)
			keys := make([]string, 0, len(proj.Tool.Poetry.DevDependencies))
			for name := range proj.Tool.Poetry.DevDependencies {
				keys = append(keys, name)
			}
			sort.Strings(keys)
			for _, name := range keys {
				if newVersion, ok := versions[name]; ok {
					devDeps[name] = "^" + newVersion
					hasChanges = true
				} else {
					devDeps[name] = proj.Tool.Poetry.DevDependencies[name]
				}
			}
			proj.Tool.Poetry.DevDependencies = devDeps
		}
	}

	// If no changes were made, return early
	if !hasChanges {
		return nil
	}

	// First pass: collect section content
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "[") {
			currentSection = trimmedLine
			seenSections[currentSection] = true
			sectionContent[currentSection] = []string{}
		} else if currentSection != "" && trimmedLine != "" {
			sectionContent[currentSection] = append(sectionContent[currentSection], line)
		}
	}

	// Second pass: write output
	currentSection = ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)

		// Check for section headers
		if strings.HasPrefix(trimmedLine, "[") {
			inDependencies = false
			inOptionalDependencies = false
			inDependencyGroups = false
			inPoetryDependencies = false
			skipUntilNextSection = false

			if trimmedLine == "[project]" {
				output = append(output, line)
				currentSection = "project"
				seenSections[currentSection] = true
				// Keep non-dependency fields
				for i++; i < len(lines); i++ {
					line = lines[i]
					trimmedLine = strings.TrimSpace(line)
					if strings.HasPrefix(trimmedLine, "[") {
						i--
						break
					}
					if !strings.HasPrefix(trimmedLine, "dependencies") && !strings.HasPrefix(trimmedLine, "]") && trimmedLine != "" && !strings.HasPrefix(trimmedLine, "\"") {
						output = append(output, line)
					}
				}
				// Add updated dependencies
				if proj.Project.Dependencies != nil {
					output = append(output, "dependencies = [")
					for i, dep := range proj.Project.Dependencies {
						if i < len(proj.Project.Dependencies)-1 {
							output = append(output, fmt.Sprintf("    %q,", dep))
						} else {
							output = append(output, fmt.Sprintf("    %q", dep))
						}
					}
					output = append(output, "]")
				}
				output = append(output, "")
				continue
			} else if trimmedLine == "[project.optional-dependencies]" {
				output = append(output, line)
				currentSection = "project.optional-dependencies"
				seenSections[currentSection] = true
				if proj.Project.OptionalDependencies != nil {
					groups := make([]string, 0, len(proj.Project.OptionalDependencies))
					for group := range proj.Project.OptionalDependencies {
						groups = append(groups, group)
					}
					sort.Strings(groups)
					for _, group := range groups {
						deps := proj.Project.OptionalDependencies[group]
						output = append(output, fmt.Sprintf("%s = [", group))
						for i, dep := range deps {
							if i < len(deps)-1 {
								output = append(output, fmt.Sprintf("    %q,", dep))
							} else {
								output = append(output, fmt.Sprintf("    %q", dep))
							}
						}
						output = append(output, "]")
					}
				}
				skipUntilNextSection = true
				output = append(output, "")
				continue
			} else if trimmedLine == "[dependency-groups]" {
				output = append(output, line)
				currentSection = "dependency-groups"
				seenSections[currentSection] = true
				if proj.DependencyGroups != nil {
					groups := make([]string, 0, len(proj.DependencyGroups))
					for group := range proj.DependencyGroups {
						groups = append(groups, group)
					}
					sort.Strings(groups)
					for _, group := range groups {
						deps := proj.DependencyGroups[group]
						output = append(output, fmt.Sprintf("%s = [", group))
						for i, dep := range deps {
							if i < len(deps)-1 {
								output = append(output, fmt.Sprintf("    %q,", dep))
							} else {
								output = append(output, fmt.Sprintf("    %q", dep))
							}
						}
						output = append(output, "]")
					}
				}
				skipUntilNextSection = true
				output = append(output, "")
				continue
			} else if trimmedLine == "[tool.poetry]" {
				output = append(output, line)
				currentSection = "tool.poetry"
				seenSections[currentSection] = true
				if proj.Tool.Poetry.Dependencies != nil {
					output = append(output, "dependencies = { ")
					keys := make([]string, 0, len(proj.Tool.Poetry.Dependencies))
					for k := range proj.Tool.Poetry.Dependencies {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for i, k := range keys {
						if i > 0 {
							output[len(output)-1] += ", "
						}
						output[len(output)-1] += fmt.Sprintf("%s = %q", k, proj.Tool.Poetry.Dependencies[k])
					}
					output[len(output)-1] += " }"
				}
				if len(proj.Tool.Poetry.DevDependencies) > 0 {
					output = append(output, "dev-dependencies = { ")
					keys := make([]string, 0, len(proj.Tool.Poetry.DevDependencies))
					for k := range proj.Tool.Poetry.DevDependencies {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for i, k := range keys {
						if i > 0 {
							output[len(output)-1] += ", "
						}
						output[len(output)-1] += fmt.Sprintf("%s = %q", k, proj.Tool.Poetry.DevDependencies[k])
					}
					output[len(output)-1] += " }"
				}
				skipUntilNextSection = true
				output = append(output, "")
				continue
			} else {
				if !seenSections[trimmedLine] {
					output = append(output, line)
					currentSection = trimmedLine
					seenSections[currentSection] = true
					if content, ok := sectionContent[currentSection]; ok {
						output = append(output, content...)
					}
				}
			}
		} else if strings.HasPrefix(trimmedLine, "dependencies") {
			inDependencies = true
			skipUntilNextSection = true
			continue
		} else if strings.HasPrefix(trimmedLine, "optional-dependencies") {
			inOptionalDependencies = true
			skipUntilNextSection = true
			continue
		} else if strings.HasPrefix(trimmedLine, "dependency-groups") {
			inDependencyGroups = true
			skipUntilNextSection = true
			continue
		} else if strings.HasPrefix(trimmedLine, "poetry") {
			inPoetryDependencies = true
			skipUntilNextSection = true
			continue
		}

		// Add non-dependency lines
		if !inDependencies && !inOptionalDependencies && !inDependencyGroups && !inPoetryDependencies && !skipUntilNextSection {
			output = append(output, line)
		}
	}

	// Remove trailing empty lines
	for len(output) > 0 && strings.TrimSpace(output[len(output)-1]) == "" {
		output = output[:len(output)-1]
	}

	// Write back to file
	return os.WriteFile(filePath, []byte(strings.Join(output, "\n")+"\n"), 0644)
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
