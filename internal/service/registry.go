// Package service provides the shared business-logic layer used by both the
// CLI and the REST server. It wraps the bankan library with per-board locking,
// consistent error types, and a registry of multiple boards.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	bankan "github.com/thekondor/bankan"
)

// BoardInfo is a summary of a registered board.
type BoardInfo struct {
	ID     string // directory basename
	Dir    string // absolute path
	IsView bool   // true when the directory contains view.md
}

// entry holds runtime state for one registered board.
type entry struct {
	dir    string
	isView bool
	order  int          // display order (0 = unset, sorts last)
	mu     sync.RWMutex // serialises concurrent writes to this board
}

// Registry manages a set of boards (regular and view) with per-board locking
// for safe concurrent access from the HTTP server. The CLI creates a
// single-entry Registry via NewSingleRegistry.
type Registry struct {
	entries map[string]*entry // key: directory basename
	rootDir string            // optional; used to create new boards
	mu      sync.RWMutex      // protects the entries map itself
}

// NewRegistry creates a Registry from a list of board directories.
// Each element of boardDirs may be either:
//   - A board/view-board directory (contains board.md or view.md), or
//   - A container directory scanned one level deep for boards.
//
// rootDir, when non-empty, is used as the parent directory for new boards
// created via InitBoard. If rootDir is empty but boardDirs contains exactly
// one container directory, that directory is used as rootDir automatically.
func NewRegistry(boardDirs []string, rootDir string) (*Registry, error) {
	r := &Registry{
		entries: make(map[string]*entry),
		rootDir: rootDir,
	}
	for _, dir := range boardDirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", dir, err)
		}
		if bankan.IsBoard(abs) || bankan.IsViewBoard(abs) {
			if _, err := r.Register(abs); err != nil {
				return nil, err
			}
		} else {
			// Treat as container: scan one level deep.
			if err := r.Scan(abs); err != nil {
				return nil, err
			}
			// Use first container as rootDir if none specified.
			if r.rootDir == "" {
				r.rootDir = abs
			}
		}
	}
	return r, nil
}

// NewSingleRegistry creates a Registry with a single board or view-board entry.
// dir must be an absolute or relative path directly to a board/view-board dir.
// Returns the Registry, the board ID (dirname basename), and any error.
func NewSingleRegistry(dir string) (*Registry, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve path %q: %w", dir, err)
	}
	r := &Registry{entries: make(map[string]*entry)}
	id, err := r.Register(abs)
	if err != nil {
		return nil, "", err
	}
	return r, id, nil
}

// Scan registers all boards and view boards found as immediate subdirectories
// of dir (one level deep).
func (r *Registry) Scan(dir string) error {
	des, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("scan %q: %w", dir, err)
	}
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		sub := filepath.Join(dir, de.Name())
		if bankan.IsBoard(sub) || bankan.IsViewBoard(sub) {
			if _, err := r.Register(sub); err != nil {
				return err
			}
		}
	}
	return nil
}

// Register adds a single board or view-board directory to the registry.
// Returns the board ID (directory basename). Returns ErrConflict if a board
// with the same basename is already registered.
func (r *Registry) Register(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", dir, err)
	}
	if !bankan.IsBoard(abs) && !bankan.IsViewBoard(abs) {
		return "", fmt.Errorf("register %q: directory is neither a board nor a view board", abs)
	}
	id := filepath.Base(abs)

	isView := bankan.IsViewBoard(abs)
	var order int
	if isView {
		if vb, err := bankan.ReadViewBoard(abs); err == nil {
			order = vb.Order
		}
	} else {
		if b, err := bankan.ReadBoard(abs); err == nil {
			order = b.Order
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[id]; exists {
		return "", &ErrConflict{Reason: fmt.Sprintf("board with ID %q (from %q) is already registered", id, abs)}
	}
	r.entries[id] = &entry{
		dir:    abs,
		isView: isView,
		order:  order,
	}
	return id, nil
}

// boardLess reports whether board i should sort before board j.
// Boards with order>0 sort first (ascending), then unordered (order==0) boards
// sort alphabetically by ID.
func boardLess(oi, oj int, idI, idJ string) bool {
	if (oi == 0) != (oj == 0) {
		return oj == 0
	}
	if oi != oj {
		return oi < oj
	}
	return idI < idJ
}

// Boards returns BoardInfo for all registered boards sorted by display order.
func (r *Registry) Boards() []BoardInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]BoardInfo, 0, len(r.entries))
	for id, e := range r.entries {
		out = append(out, BoardInfo{ID: id, Dir: e.dir, IsView: e.isView})
	}
	orderOf := func(id string) int { return r.entries[id].order }
	sort.Slice(out, func(i, j int) bool {
		return boardLess(orderOf(out[i].ID), orderOf(out[j].ID), out[i].ID, out[j].ID)
	})
	return out
}

// BoardIDs returns all registered board IDs sorted by display order.
func (r *Registry) BoardIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	type idOrder struct {
		id    string
		order int
	}
	items := make([]idOrder, 0, len(r.entries))
	for id, e := range r.entries {
		items = append(items, idOrder{id: id, order: e.order})
	}
	sort.Slice(items, func(i, j int) bool {
		return boardLess(items[i].order, items[j].order, items[i].id, items[j].id)
	})
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.id
	}
	return ids
}

// IsViewBoard reports whether the board with id is a view board.
func (r *Registry) IsViewBoard(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	return ok && e.isView
}

// IsArchivedViewBoard reports whether the board with id is an archived view board.
// Returns false if the board does not exist, is not a view board, or cannot be read.
func (r *Registry) IsArchivedViewBoard(id string) bool {
	r.mu.RLock()
	e, ok := r.entries[id]
	r.mu.RUnlock()
	if !ok || !e.isView {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	vb, err := bankan.ReadViewBoard(e.dir)
	if err != nil {
		return false
	}
	return vb.ArchivedAt != nil
}

// RootDir returns the root directory used for board creation (may be "").
func (r *Registry) RootDir() string { return r.rootDir }

// ReorderBoards persists a new display order for all boards.
// ids must contain every registered board ID exactly once; the slice position
// (0-based) determines the new order value written to each board's frontmatter.
func (r *Registry) ReorderBoards(ids []string) error {
	r.mu.RLock()
	entries := make([]*entry, 0, len(ids))
	for _, id := range ids {
		e, ok := r.entries[id]
		if !ok {
			r.mu.RUnlock()
			return &ErrNotFound{Resource: "board", ID: id}
		}
		entries = append(entries, e)
	}
	r.mu.RUnlock()

	for i, e := range entries {
		order := i + 1
		e.mu.Lock()
		var writeErr error
		if e.isView {
			vb, err := bankan.ReadViewBoard(e.dir)
			if err != nil {
				e.mu.Unlock()
				return fmt.Errorf("reorder boards: read view board: %w", err)
			}
			vb.Order = order
			writeErr = bankan.WriteViewBoard(vb)
		} else {
			b, err := bankan.ReadBoard(e.dir)
			if err != nil {
				e.mu.Unlock()
				return fmt.Errorf("reorder boards: read board: %w", err)
			}
			b.Order = order
			writeErr = bankan.WriteBoard(b)
		}
		if writeErr != nil {
			e.mu.Unlock()
			return fmt.Errorf("reorder boards: write: %w", writeErr)
		}
		e.order = order
		e.mu.Unlock()
	}
	return nil
}

// nextBoardOrder returns max(existing orders) + 1 for new board creation.
func (r *Registry) nextBoardOrder() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	max := 0
	for _, e := range r.entries {
		if e.order > max {
			max = e.order
		}
	}
	return max + 1
}

// ─── internal helpers ────────────────────────────────────────────────────────

func (r *Registry) getEntry(id string) (*entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, &ErrNotFound{Resource: "board", ID: id}
	}
	return e, nil
}

// withReadLock acquires the per-board read lock, runs fn, releases it.
func (r *Registry) withReadLock(id string, fn func(dir string, isView bool) error) error {
	e, err := r.getEntry(id)
	if err != nil {
		return err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return fn(e.dir, e.isView)
}

// withWriteLock acquires the per-board write lock, runs fn, releases it.
func (r *Registry) withWriteLock(id string, fn func(dir string, isView bool) error) error {
	e, err := r.getEntry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return fn(e.dir, e.isView)
}
