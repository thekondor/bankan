package bankan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AddViewLane adds a new lane to a view board, enforcing name uniqueness
// across both the view's own lanes AND the parent board's lanes.
//
// The new lane is view-local: it is not created in the parent board and
// will not affect the parent.
func AddViewLane(vb *ViewBoard, parent *Board, name string) (Lane, error) {
	if strings.TrimSpace(name) == "" {
		return Lane{}, fmt.Errorf("add view lane: name must not be empty")
	}

	viewLanes, err := ReadLanes(vb.Dir)
	if err != nil {
		return Lane{}, fmt.Errorf("add view lane: %w", err)
	}
	parentLanes, err := ReadLanes(parent.Dir)
	if err != nil {
		return Lane{}, fmt.Errorf("add view lane: %w", err)
	}

	// Uniqueness check across the combined set.
	if _, exists := LaneByName(viewLanes, name); exists {
		return Lane{}, fmt.Errorf("add view lane: lane %q already exists in view", name)
	}
	if _, exists := LaneByName(parentLanes, name); exists {
		return Lane{}, fmt.Errorf("add view lane: lane %q already exists in parent board", name)
	}

	maxOrder := 0
	for _, l := range viewLanes {
		if l.Order > maxOrder {
			maxOrder = l.Order
		}
	}
	order := maxOrder + 1

	dirName := laneDirName(order, name)
	fullPath := filepath.Join(vb.Dir, dirName)
	if err := os.Mkdir(fullPath, 0o755); err != nil {
		return Lane{}, fmt.Errorf("add view lane: mkdir: %w", err)
	}

	return Lane{Name: deslugify(slugify(name)), Dir: fullPath, Order: order}, nil
}

// RemoveViewLane removes a lane from the view board only.
// Any card stubs inside the lane are deleted first; the parent board is
// never touched (stubs are ephemeral view pointers).
func RemoveViewLane(vb *ViewBoard, name string) error {
	lanes, err := ReadLanes(vb.Dir)
	if err != nil {
		return fmt.Errorf("remove view lane: %w", err)
	}

	lane, exists := LaneByName(lanes, name)
	if !exists {
		return fmt.Errorf("remove view lane: lane %q not found in view", name)
	}

	// Remove all stub files inside the lane directory before removing the dir.
	entries, err := os.ReadDir(lane.Dir)
	if err != nil {
		return fmt.Errorf("remove view lane: read dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			if err := os.Remove(filepath.Join(lane.Dir, e.Name())); err != nil {
				return fmt.Errorf("remove view lane: remove stub %q: %w", e.Name(), err)
			}
		}
	}

	if err := os.Remove(lane.Dir); err != nil {
		return fmt.Errorf("remove view lane: %w", err)
	}
	return nil
}
