package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Workspace groups a set of boards from a single root directory.
type Workspace struct {
	ID   string    // URL-safe slug derived from Name after collision resolution
	Name string    // display name (e.g. "b/myproject" or "foo (1)")
	Reg  *Registry
}

// WorkspaceArg is parsed from a CLI positional argument (name:path or path).
type WorkspaceArg struct {
	Name string // explicit name; empty means derive from Dir
	Dir  string
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// workspaceSlug converts a display name to a URL-safe slug.
func workspaceSlug(name string) string {
	s := strings.ToLower(name)
	s = nonAlphanumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "workspace"
	}
	return s
}

// DerivedWorkspaceName returns the last two path components of absDir joined
// with "/" as the default workspace display name,
// e.g. "/a/b/myproject" → "b/myproject".
func DerivedWorkspaceName(absDir string) string {
	base := filepath.Base(absDir)
	parent := filepath.Base(filepath.Dir(absDir))
	if parent == "." || parent == "" || parent == string(filepath.Separator) {
		return base
	}
	return parent + "/" + base
}

// NewWorkspaces creates Workspace instances from CLI args.
// It handles name derivation, collision resolution (all colliders get "(n)"
// suffixes sorted by Dir), and unique slug generation.
// When args is empty, a single workspace from cwd is used.
func NewWorkspaces(args []WorkspaceArg) ([]*Workspace, error) {
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get cwd: %w", err)
		}
		args = []WorkspaceArg{{Dir: cwd}}
	}

	type resolved struct {
		name string
		dir  string
	}
	items := make([]resolved, 0, len(args))
	for _, a := range args {
		abs, err := filepath.Abs(a.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", a.Dir, err)
		}
		name := a.Name
		if name == "" {
			name = DerivedWorkspaceName(abs)
		}
		items = append(items, resolved{name: name, dir: abs})
	}

	// Detect name collisions and apply "(n)" suffixes to ALL colliders.
	// Group by name, sort each group by dir for stable ordering.
	type group struct {
		indices []int
	}
	byName := make(map[string]*group)
	for i, it := range items {
		g := byName[it.name]
		if g == nil {
			g = &group{}
			byName[it.name] = g
		}
		g.indices = append(g.indices, i)
	}
	finalNames := make([]string, len(items))
	for i, it := range items {
		finalNames[i] = it.name
	}
	for name, g := range byName {
		if len(g.indices) < 2 {
			continue
		}
		sort.Slice(g.indices, func(a, b int) bool {
			return items[g.indices[a]].dir < items[g.indices[b]].dir
		})
		for n, idx := range g.indices {
			finalNames[idx] = fmt.Sprintf("%s (%d)", name, n+1)
		}
	}

	// Derive slugs; ensure slug uniqueness in case two distinct names produce
	// the same slug (e.g. "foo-bar" and "foo bar" both → "foo-bar").
	rawSlugs := make([]string, len(items))
	for i := range items {
		rawSlugs[i] = workspaceSlug(finalNames[i])
	}
	slugCount := make(map[string]int)
	for _, s := range rawSlugs {
		slugCount[s]++
	}
	slugUsed := make(map[string]int)
	slugs := make([]string, len(items))
	for i, s := range rawSlugs {
		if slugCount[s] > 1 {
			slugUsed[s]++
			slugs[i] = fmt.Sprintf("%s-%d", s, slugUsed[s])
		} else {
			slugs[i] = s
		}
	}

	// Build one Registry per workspace.
	workspaces := make([]*Workspace, len(items))
	for i, it := range items {
		reg, err := NewRegistry([]string{it.dir}, "")
		if err != nil {
			return nil, fmt.Errorf("workspace %q: %w", finalNames[i], err)
		}
		workspaces[i] = &Workspace{
			ID:   slugs[i],
			Name: finalNames[i],
			Reg:  reg,
		}
	}
	return workspaces, nil
}
