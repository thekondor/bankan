package bankan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const archiveDirName = "_archive"

// Lane represents a column in a kanban board.
// Its directory name follows the pattern NN-<slug> where NN is a zero-padded
// two-digit ordering prefix.
type Lane struct {
	Name  string // display name, e.g. "Backlog"
	Dir   string // absolute path to the lane directory
	Order int    // numeric prefix value, e.g. 1 for "01-backlog"
}

// laneNameRe matches directory names of the form "NN-<rest>" where NN >= 1.
var laneNameRe = regexp.MustCompile(`^(\d{2})-(.+)$`)

// parseLaneDir extracts order and display name from a directory base name.
func parseLaneDir(base string) (order int, name string, ok bool) {
	m := laneNameRe.FindStringSubmatch(base)
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0, "", false
	}
	return n, deslugify(m[2]), true
}

// laneDirName builds the directory base name from order and a display name.
func laneDirName(order int, name string) string {
	return fmt.Sprintf("%02d-%s", order, slugify(name))
}

// Slugify converts a display name to a safe directory slug.
// It is exported so that the service layer and CLI can derive directory names
// consistently with the same algorithm used internally for lane directories.
func Slugify(s string) string { return slugify(s) }

// Deslugify converts a slug back to a display name (hyphens → spaces).
// It is exported for symmetry with Slugify.
func Deslugify(s string) string { return deslugify(s) }

// slugify converts a display name to a safe directory slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteByte('-')
		}
	}
	// Collapse multiple dashes.
	result := regexp.MustCompile(`-{2,}`).ReplaceAllString(b.String(), "-")
	return strings.Trim(result, "-")
}

// deslugify converts a slug back to a display name (hyphens → spaces, title case first word).
func deslugify(s string) string {
	return strings.ReplaceAll(s, "-", " ")
}

// ReadLanes returns all lanes in the board directory, sorted by order prefix.
func ReadLanes(boardDir string) ([]Lane, error) {
	entries, err := os.ReadDir(boardDir)
	if err != nil {
		return nil, fmt.Errorf("read lanes: %w", err)
	}

	var lanes []Lane
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		order, name, ok := parseLaneDir(e.Name())
		if !ok {
			continue
		}
		lanes = append(lanes, Lane{
			Name:  name,
			Dir:   filepath.Join(boardDir, e.Name()),
			Order: order,
		})
	}

	sort.Slice(lanes, func(i, j int) bool {
		return lanes[i].Order < lanes[j].Order
	})
	return lanes, nil
}

// LaneByName finds a lane by display name (case-insensitive).
func LaneByName(lanes []Lane, name string) (Lane, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, l := range lanes {
		if strings.ToLower(l.Name) == lower {
			return l, true
		}
	}
	return Lane{}, false
}

// AddLane creates a new lane at the end of the current lane list.
func AddLane(b *Board, name string) (Lane, error) {
	if strings.TrimSpace(name) == "" {
		return Lane{}, errors.New("add lane: name must not be empty")
	}

	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return Lane{}, err
	}

	// Check uniqueness (case-insensitive).
	if _, exists := LaneByName(lanes, name); exists {
		return Lane{}, fmt.Errorf("add lane: lane %q already exists", name)
	}

	// Determine next order.
	maxOrder := 0
	for _, l := range lanes {
		if l.Order > maxOrder {
			maxOrder = l.Order
		}
	}
	order := maxOrder + 1

	dirName := laneDirName(order, name)
	fullPath := filepath.Join(b.Dir, dirName)
	if err := os.Mkdir(fullPath, 0o755); err != nil {
		return Lane{}, fmt.Errorf("add lane: mkdir: %w", err)
	}

	return Lane{Name: deslugify(slugify(name)), Dir: fullPath, Order: order}, nil
}

// RenameLane renames a lane, preserving its order prefix.
func RenameLane(b *Board, oldName, newName string) error {
	if strings.TrimSpace(newName) == "" {
		return errors.New("rename lane: new name must not be empty")
	}

	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return err
	}

	lane, exists := LaneByName(lanes, oldName)
	if !exists {
		return fmt.Errorf("rename lane: lane %q not found", oldName)
	}

	if _, conflict := LaneByName(lanes, newName); conflict {
		return fmt.Errorf("rename lane: lane %q already exists", newName)
	}

	newDirName := laneDirName(lane.Order, newName)
	newPath := filepath.Join(b.Dir, newDirName)
	if err := os.Rename(lane.Dir, newPath); err != nil {
		return fmt.Errorf("rename lane: %w", err)
	}
	return nil
}

// ReorderLanes reassigns order prefixes so that lanes appear in the given name
// order. names must be a permutation of the current lane display names
// (case-insensitive comparison). The function is a no-op if the order is
// already correct.
//
// Implementation uses a two-pass rename (to temp names, then to final names)
// to avoid directory-name collisions during the swap.
func ReorderLanes(b *Board, names []string) error {
	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return fmt.Errorf("reorder lanes: %w", err)
	}

	if len(names) != len(lanes) {
		return fmt.Errorf("reorder lanes: expected %d lane names, got %d", len(lanes), len(names))
	}

	// Build lookup map: lower-case name → Lane.
	laneMap := make(map[string]Lane, len(lanes))
	for _, l := range lanes {
		laneMap[strings.ToLower(l.Name)] = l
	}

	// Validate and resolve the desired order.
	desired := make([]Lane, len(names))
	seen := make(map[string]bool, len(names))
	for i, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return fmt.Errorf("reorder lanes: empty name at position %d", i)
		}
		l, ok := laneMap[key]
		if !ok {
			return fmt.Errorf("reorder lanes: lane %q not found", name)
		}
		if seen[key] {
			return fmt.Errorf("reorder lanes: duplicate lane %q", name)
		}
		seen[key] = true
		desired[i] = l
	}

	// No-op check: already in target order.
	alreadyOrdered := true
	for i, l := range desired {
		if l.Order != i+1 {
			alreadyOrdered = false
			break
		}
	}
	if alreadyOrdered {
		return nil
	}

	// Pass 1: rename every lane directory to a temporary name that cannot
	// collide with any NN-slug pattern (underscore prefix is not matched by
	// laneNameRe).
	tmpPaths := make([]string, len(desired))
	for i, l := range desired {
		tmp := filepath.Join(b.Dir, fmt.Sprintf("_reorder_%d_", i))
		if err := os.Rename(l.Dir, tmp); err != nil {
			return fmt.Errorf("reorder lanes: temp rename: %w", err)
		}
		tmpPaths[i] = tmp
	}

	// Pass 2: rename from temp names to final NN-slug names.
	for i, l := range desired {
		final := filepath.Join(b.Dir, laneDirName(i+1, l.Name))
		if err := os.Rename(tmpPaths[i], final); err != nil {
			return fmt.Errorf("reorder lanes: final rename: %w", err)
		}
	}

	return nil
}

// RemoveLane removes a lane directory. Returns an error if it contains any cards.
func RemoveLane(b *Board, name string) error {
	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return err
	}

	lane, exists := LaneByName(lanes, name)
	if !exists {
		return fmt.Errorf("remove lane: lane %q not found", name)
	}

	// Check for cards (any .md file in the directory).
	entries, err := os.ReadDir(lane.Dir)
	if err != nil {
		return fmt.Errorf("remove lane: read dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			return fmt.Errorf("remove lane: lane %q is not empty", name)
		}
	}

	if err := os.Remove(lane.Dir); err != nil {
		return fmt.Errorf("remove lane: %w", err)
	}
	return nil
}
