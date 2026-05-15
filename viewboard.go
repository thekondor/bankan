package bankan

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const viewFileName = "view.md"

// ViewBoard is a label-filtered subset view of a parent Board.
//
// A directory is a view board if and only if it contains view.md.
// It is mutually exclusive with board.md — IsViewBoard and IsBoard
// never both return true for the same directory.
//
// The FilterLabel field is frozen at creation time and can never be changed.
// All card data lives in the parent board's directory; the view board holds
// only stub files that reference parent cards and its own lane structure.
type ViewBoard struct {
	Name        string     `yaml:"name"`
	Order       int        `yaml:"order,omitempty"`
	Hidden      bool       `yaml:"hidden,omitempty"`
	Parent      string     `yaml:"parent"`       // absolute path to parent board directory
	FilterLabel string     `yaml:"filter_label"` // label ID on the parent board (immutable)
	CreatedAt   time.Time  `yaml:"created_at"`
	ArchivedAt  *time.Time `yaml:"archived_at,omitempty"`
	Color       string     `yaml:"color,omitempty"` // accent/background color (hex, e.g. "#4ade80")

	// Body is the optional markdown description that follows the frontmatter.
	Body string `yaml:"-"`
	// Dir is the absolute path to the view board's root directory (runtime only).
	Dir string `yaml:"-"`
}

// viewFilePath returns the path to view.md inside the given directory.
func viewFilePath(dir string) string {
	return filepath.Join(dir, viewFileName)
}

// IsViewBoard reports whether dir contains a view.md file.
func IsViewBoard(dir string) bool {
	_, err := os.Stat(viewFilePath(dir))
	return err == nil
}

// InitViewBoard creates a new view board in dir.
//
// parentDir must be an absolute path to an existing board (board.md must be present).
// filterLabelID must be a label ID that exists on the parent board.
// The parent board's lanes are cloned into the view board at creation time.
// Returns an error if view.md already exists, or if the parent or label are invalid.
func InitViewBoard(dir, name, parentDir, filterLabelID string) (*ViewBoard, error) {
	absParent, err := filepath.Abs(parentDir)
	if err != nil {
		return nil, fmt.Errorf("init view board: resolve parent path: %w", err)
	}
	if !IsBoard(absParent) {
		return nil, fmt.Errorf("init view board: %q is not a board directory", absParent)
	}

	parent, err := ReadBoard(absParent)
	if err != nil {
		return nil, fmt.Errorf("init view board: read parent: %w", err)
	}
	if _, ok := FindLabelByID(parent.Labels, filterLabelID); !ok {
		return nil, fmt.Errorf("init view board: label %q not found on parent board", filterLabelID)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("init view board: create dir: %w", err)
	}

	vfp := viewFilePath(dir)
	if _, err := os.Stat(vfp); err == nil {
		return nil, fmt.Errorf("init view board: view.md already exists in %s", dir)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("init view board: resolve dir path: %w", err)
	}

	vb := &ViewBoard{
		Name:        name,
		Parent:      absParent,
		FilterLabel: filterLabelID,
		CreatedAt:   time.Now().UTC(),
		Color:       boardColorPalette[rand.Intn(len(boardColorPalette))],
		Dir:         absDir,
	}

	if err := WriteViewBoard(vb); err != nil {
		return nil, err
	}

	// Create _archive directory in the view board (unused but kept consistent
	// with regular board structure so lane scanning tools work uniformly).
	if err := os.MkdirAll(filepath.Join(absDir, archiveDirName), 0o755); err != nil {
		return nil, fmt.Errorf("init view board: create archive dir: %w", err)
	}

	// Clone parent lanes into the view board.
	parentLanes, err := ReadLanes(absParent)
	if err != nil {
		return nil, fmt.Errorf("init view board: read parent lanes: %w", err)
	}
	for _, pl := range parentLanes {
		dirName := laneDirName(pl.Order, pl.Name)
		if err := os.Mkdir(filepath.Join(absDir, dirName), 0o755); err != nil {
			return nil, fmt.Errorf("init view board: clone lane %q: %w", pl.Name, err)
		}
	}

	return vb, nil
}

// ReadViewBoard reads and parses view.md from dir.
func ReadViewBoard(dir string) (*ViewBoard, error) {
	data, err := os.ReadFile(viewFilePath(dir))
	if err != nil {
		return nil, fmt.Errorf("read view board: %w", err)
	}

	var vb ViewBoard
	body, err := Parse(data, &vb)
	if err != nil {
		return nil, fmt.Errorf("read view board: %w", err)
	}
	vb.Body = body
	vb.Dir = dir
	return &vb, nil
}

// WriteViewBoard serializes and writes view.md for vb.
func WriteViewBoard(vb *ViewBoard) error {
	data, err := Serialize(vb, vb.Body)
	if err != nil {
		return fmt.Errorf("write view board: %w", err)
	}
	if err := os.WriteFile(viewFilePath(vb.Dir), data, 0o644); err != nil {
		return fmt.Errorf("write view board: %w", err)
	}
	return nil
}

// ArchiveViewBoard marks a view board as archived by setting ArchivedAt in view.md.
// This does not affect any cards in the parent board.
func ArchiveViewBoard(vb *ViewBoard) error {
	if vb.ArchivedAt != nil {
		return errors.New("archive view board: view board is already archived")
	}
	now := time.Now().UTC()
	vb.ArchivedAt = &now
	return WriteViewBoard(vb)
}

// FindViewBoard walks up the directory tree starting at startDir looking for a
// directory that contains view.md. Returns an error if none is found.
func FindViewBoard(startDir string) (*ViewBoard, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("find view board: %w", err)
	}

	for {
		if IsViewBoard(dir) {
			return ReadViewBoard(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, errors.New("find view board: no view.md found in directory tree")
}

// ParentBoard loads and returns the parent Board of a view board.
func ParentBoard(vb *ViewBoard) (*Board, error) {
	pb, err := ReadBoard(vb.Parent)
	if err != nil {
		return nil, fmt.Errorf("load parent board: %w", err)
	}
	return pb, nil
}

// UpdateViewBoardColor sets the accent color on a view board and writes view.md.
func UpdateViewBoardColor(vb *ViewBoard, color string) error {
	if !hexColorRe.MatchString(color) {
		return fmt.Errorf("update view board color: %q is not a valid hex color (e.g. \"#4ade80\")", color)
	}
	vb.Color = color
	return WriteViewBoard(vb)
}

// HideViewBoard sets hidden:true on the view board and writes view.md.
func HideViewBoard(vb *ViewBoard) error {
	vb.Hidden = true
	return WriteViewBoard(vb)
}

// ShowViewBoard clears hidden on the view board and writes view.md.
func ShowViewBoard(vb *ViewBoard) error {
	vb.Hidden = false
	return WriteViewBoard(vb)
}
