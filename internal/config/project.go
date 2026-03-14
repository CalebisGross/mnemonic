package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectConfig defines a canonical project with path mappings and aliases.
type ProjectConfig struct {
	Name    string   `yaml:"name"`
	Paths   []string `yaml:"paths"`
	Aliases []string `yaml:"aliases"`
}

// ProjectResolver maps paths, directory names, and aliases to canonical project names.
type ProjectResolver struct {
	pathMap  []pathEntry       // sorted longest-first for prefix matching
	aliasMap map[string]string // lowercase alias/name -> canonical name
}

type pathEntry struct {
	prefix string
	name   string
}

// knownProjectParents are directory names that typically contain project directories.
// This is the fallback heuristic when no config-based match is found.
var knownProjectParents = map[string]bool{
	"Projects":  true,
	"projects":  true,
	"src":       true,
	"repos":     true,
	"workspace": true,
	"Workspace": true,
}

// NewProjectResolver builds a resolver from project configs.
// Paths should already be expanded (~ resolved) before calling this.
func NewProjectResolver(projects []ProjectConfig) *ProjectResolver {
	pr := &ProjectResolver{
		aliasMap: make(map[string]string),
	}

	for _, p := range projects {
		canonical := p.Name

		// Register canonical name as an alias for itself
		pr.aliasMap[strings.ToLower(canonical)] = canonical

		// Register explicit aliases
		for _, alias := range p.Aliases {
			pr.aliasMap[strings.ToLower(alias)] = canonical
		}

		// Register path prefixes
		for _, path := range p.Paths {
			clean := filepath.Clean(path)
			pr.pathMap = append(pr.pathMap, pathEntry{prefix: clean, name: canonical})
		}
	}

	// Sort path entries longest-first so longer (more specific) paths match before shorter ones
	for i := 0; i < len(pr.pathMap); i++ {
		for j := i + 1; j < len(pr.pathMap); j++ {
			if len(pr.pathMap[j].prefix) > len(pr.pathMap[i].prefix) {
				pr.pathMap[i], pr.pathMap[j] = pr.pathMap[j], pr.pathMap[i]
			}
		}
	}

	return pr
}

// Resolve maps an input to a canonical project name.
//
// Resolution priority:
//  1. Exact alias/name match (case-insensitive) — handles explicit input like "mem"
//  2. Path prefix match (longest wins) — handles file paths like "/home/user/Projects/mem/foo.go"
//  3. Fallback heuristic — walks path looking for known parent dirs (Projects, src, etc.)
//  4. Empty string if nothing matches
func (pr *ProjectResolver) Resolve(input string) string {
	if input == "" {
		return ""
	}

	// 1. Exact alias/name match (handles bare names like "mem", "sdk")
	if canonical, ok := pr.aliasMap[strings.ToLower(input)]; ok {
		return canonical
	}

	// Expand ~ for path-based checks
	expanded := input
	if strings.HasPrefix(expanded, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[1:])
		}
	}
	expanded = filepath.Clean(expanded)

	// 2. Path prefix match (longest registered path wins)
	for _, entry := range pr.pathMap {
		if strings.HasPrefix(expanded, entry.prefix) {
			// Ensure we match at a path boundary (not partial directory name)
			if len(expanded) == len(entry.prefix) || expanded[len(entry.prefix)] == os.PathSeparator {
				return entry.name
			}
		}
	}

	// 3. Fallback: known project parent heuristic
	parts := strings.Split(expanded, string(os.PathSeparator))
	for i, part := range parts {
		if knownProjectParents[part] && i+1 < len(parts) {
			child := parts[i+1]
			// Check if the inferred child matches an alias
			if canonical, ok := pr.aliasMap[strings.ToLower(child)]; ok {
				return canonical
			}
			return child
		}
	}

	return ""
}
