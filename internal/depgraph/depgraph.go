package depgraph

import (
	"fmt"
	"sort"
	"strings"

	semv "github.com/Masterminds/semver/v3"
)

// Node represents a package in the dependency graph
type Node struct {
	Name           string
	CurrentVersion string
	LatestVersion  string
	Dependencies   map[string]*Node
	RequiredBy     map[string]*Node
	Constraints    []string // Version constraints from different dependents
}

// Graph represents the dependency graph
type Graph struct {
	Nodes map[string]*Node
}

// New creates a new dependency graph
func New() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
	}
}

// AddNode adds or retrieves a node in the graph
func (g *Graph) AddNode(name, version string) *Node {
	if node, exists := g.Nodes[name]; exists {
		return node
	}
	node := &Node{
		Name:           name,
		CurrentVersion: version,
		Dependencies:   make(map[string]*Node),
		RequiredBy:     make(map[string]*Node),
		Constraints:    make([]string, 0),
	}
	g.Nodes[name] = node
	return node
}

// AddDependency adds a dependency relationship between two packages
func (g *Graph) AddDependency(from, to string, constraint string) error {
	fromNode := g.AddNode(from, "")
	toNode := g.AddNode(to, "")

	fromNode.Dependencies[to] = toNode
	toNode.RequiredBy[from] = fromNode
	toNode.Constraints = append(toNode.Constraints, constraint)

	return nil
}

// DetectCycles finds any circular dependencies in the graph
func (g *Graph) DetectCycles() [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	path := make(map[string]bool)

	var visit func(node *Node, current []string)
	visit = func(node *Node, current []string) {
		if path[node.Name] {
			// Found a cycle
			cycleStart := -1
			for i, pkg := range current {
				if pkg == node.Name {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := append(current[cycleStart:], node.Name)
				// Normalize cycle to start with the alphabetically first package
				firstIdx := 0
				for i, pkg := range cycle {
					if pkg < cycle[firstIdx] {
						firstIdx = i
					}
				}
				if firstIdx > 0 {
					// Rotate the cycle to start with the alphabetically first package
					cycle = append(cycle[firstIdx:], cycle[:firstIdx]...)
				}
				cycles = append(cycles, cycle)
			}
			return
		}

		if visited[node.Name] {
			return
		}

		visited[node.Name] = true
		path[node.Name] = true
		current = append(current, node.Name)

		// Visit dependencies in a deterministic order
		deps := make([]string, 0, len(node.Dependencies))
		for dep := range node.Dependencies {
			deps = append(deps, dep)
		}
		sort.Strings(deps)

		for _, dep := range deps {
			visit(node.Dependencies[dep], current)
		}

		path[node.Name] = false
	}

	// Start from each node to ensure we find all cycles
	nodes := make([]string, 0, len(g.Nodes))
	for name := range g.Nodes {
		nodes = append(nodes, name)
	}
	sort.Strings(nodes)

	for _, name := range nodes {
		if !visited[name] {
			visit(g.Nodes[name], nil)
		}
	}

	return cycles
}

// ValidateUpdate checks if a proposed version update satisfies all constraints
func (g *Graph) ValidateUpdate(name, newVersion string) error {
	node, exists := g.Nodes[name]
	if !exists {
		return fmt.Errorf("package %s not found in graph", name)
	}

	v, err := semv.NewVersion(newVersion)
	if err != nil {
		return fmt.Errorf("invalid version %s: %w", newVersion, err)
	}

	// Check each constraint
	for _, constraint := range node.Constraints {
		// Skip empty constraints
		if constraint == "" {
			continue
		}

		// Handle different constraint formats
		var c *semv.Constraints
		if strings.HasPrefix(constraint, "~=") {
			// Handle PEP 440 compatible release operator
			baseVersion := strings.TrimPrefix(constraint, "~=")
			parts := strings.Split(baseVersion, ".")
			if len(parts) >= 2 {
				// ~=2.2 means >=2.2,<3.0
				c, err = semv.NewConstraint(fmt.Sprintf(">=%s,<%d.0", baseVersion, semv.MustParse(parts[0]).Major()+1))
			} else {
				return fmt.Errorf("invalid compatible release constraint: %s", constraint)
			}
		} else if strings.HasPrefix(constraint, "==") {
			// For exact version constraints, we want to allow updates to newer versions
			currentVersion := strings.TrimPrefix(constraint, "==")
			cv, err := semv.NewVersion(currentVersion)
			if err != nil {
				return fmt.Errorf("invalid version in constraint %s: %w", constraint, err)
			}
			// Allow the update if the new version is greater
			if !v.GreaterThan(cv) {
				return fmt.Errorf("new version %s is not greater than current version %s", newVersion, currentVersion)
			}
			continue
		} else {
			c, err = semv.NewConstraint(constraint)
		}
		if err != nil {
			return fmt.Errorf("invalid constraint %s: %w", constraint, err)
		}

		if !c.Check(v) {
			return fmt.Errorf("version %s violates constraint %s", newVersion, constraint)
		}
	}

	return nil
}

// GetUpdateOrder returns packages in dependency order for updates
func (g *Graph) GetUpdateOrder() []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(node *Node)
	visit = func(node *Node) {
		if visited[node.Name] {
			return
		}
		visited[node.Name] = true

		// Visit dependencies first
		deps := make([]string, 0, len(node.Dependencies))
		for dep := range node.Dependencies {
			deps = append(deps, dep)
		}
		sort.Strings(deps)

		for _, dep := range deps {
			visit(g.Nodes[dep])
		}

		order = append(order, node.Name)
	}

	// Start with packages that have no dependents
	roots := make([]string, 0)
	for name, node := range g.Nodes {
		if len(node.RequiredBy) == 0 {
			roots = append(roots, name)
		}
	}
	sort.Strings(roots)

	for _, root := range roots {
		visit(g.Nodes[root])
	}

	return order
}
