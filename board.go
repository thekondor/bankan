package bankan

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var boardColorPalette = []string{
	"#4ade80", "#60a5fa", "#f472b6", "#fb923c", "#a78bfa",
	"#34d399", "#facc15", "#38bdf8", "#f87171", "#e879f9",
}

const boardFileName = "board.md"

// Board represents a kanban board. Its root directory contains board.md,
// lane directories, and the _archive directory.
type Board struct {
	Name      string    `yaml:"name"`
	Order     int       `yaml:"order,omitempty"`
	Hidden    bool      `yaml:"hidden,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	Color     string    `yaml:"color,omitempty"`
	Labels    []Label   `yaml:"labels,omitempty"`

	// Body is the optional markdown description that follows the frontmatter.
	Body string `yaml:"-"`
	// Dir is the absolute path to the board's root directory (runtime only).
	Dir string `yaml:"-"`
}

// boardFilePath returns the path to board.md inside the given directory.
func boardFilePath(dir string) string {
	return filepath.Join(dir, boardFileName)
}

// IsBoard reports whether dir contains a board.md file.
func IsBoard(dir string) bool {
	_, err := os.Stat(boardFilePath(dir))
	return err == nil
}

// InitBoard creates a new board in dir. It writes board.md and creates the
// _archive directory. Returns an error if board.md already exists.
func InitBoard(dir, name string) (*Board, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("init board: create dir: %w", err)
	}

	bfp := boardFilePath(dir)
	if _, err := os.Stat(bfp); err == nil {
		return nil, fmt.Errorf("init board: board.md already exists in %s", dir)
	}

	b := &Board{
		Name:      name,
		CreatedAt: time.Now().UTC(),
		Color:     boardColorPalette[rand.Intn(len(boardColorPalette))],
		Dir:       dir,
	}

	if err := WriteBoard(b); err != nil {
		return nil, err
	}

	// Create the archive directory up front so it is always present.
	if err := os.MkdirAll(archiveDir(dir), 0o755); err != nil {
		return nil, fmt.Errorf("init board: create archive dir: %w", err)
	}

	return b, nil
}

// ReadBoard reads and parses board.md from dir.
func ReadBoard(dir string) (*Board, error) {
	data, err := os.ReadFile(boardFilePath(dir))
	if err != nil {
		return nil, fmt.Errorf("read board: %w", err)
	}

	var b Board
	body, err := Parse(data, &b)
	if err != nil {
		return nil, fmt.Errorf("read board: %w", err)
	}
	b.Body = body
	b.Dir = dir
	return &b, nil
}

// WriteBoard serializes and writes board.md for b.
func WriteBoard(b *Board) error {
	data, err := Serialize(b, b.Body)
	if err != nil {
		return fmt.Errorf("write board: %w", err)
	}
	if err := os.WriteFile(boardFilePath(b.Dir), data, 0o644); err != nil {
		return fmt.Errorf("write board: %w", err)
	}
	return nil
}

// FindBoard walks up the directory tree starting at startDir looking for a
// directory that contains board.md. Returns an error if none is found.
func FindBoard(startDir string) (*Board, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("find board: %w", err)
	}

	for {
		if IsBoard(dir) {
			return ReadBoard(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, errors.New("find board: no board.md found in directory tree")
}

// archiveDir returns the path to the _archive directory inside a board.
func archiveDir(boardDir string) string {
	return filepath.Join(boardDir, "_archive")
}

// AddLabel adds a label to the board, validates uniqueness, and writes board.md.
func AddLabel(b *Board, l Label) error {
	candidate := append(append([]Label{}, b.Labels...), l)
	if err := ValidateLabels(candidate); err != nil {
		return fmt.Errorf("add label: %w", err)
	}
	b.Labels = candidate
	return WriteBoard(b)
}

// UpdateLabel replaces the label with l.ID, validates, and writes board.md.
func UpdateLabel(b *Board, l Label) error {
	idx := -1
	for i, existing := range b.Labels {
		if existing.ID == l.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("update label: label %q not found", l.ID)
	}

	next := append([]Label{}, b.Labels...)
	next[idx] = l
	if err := ValidateLabels(next); err != nil {
		return fmt.Errorf("update label: %w", err)
	}
	b.Labels = next
	return WriteBoard(b)
}

// UpdateBoardColor sets the board's primary accent color and writes board.md.
func UpdateBoardColor(b *Board, color string) error {
	if !hexColorRe.MatchString(color) {
		return fmt.Errorf("update board color: %q is not a valid hex color (e.g. \"#4ade80\")", color)
	}
	b.Color = color
	return WriteBoard(b)
}

// HideBoard sets hidden:true on the board and writes board.md.
func HideBoard(b *Board) error {
	b.Hidden = true
	return WriteBoard(b)
}

// ShowBoard clears hidden on the board and writes board.md.
func ShowBoard(b *Board) error {
	b.Hidden = false
	return WriteBoard(b)
}

// RemoveLabel removes the label with the given ID and writes board.md.
func RemoveLabel(b *Board, id string) error {
	next := make([]Label, 0, len(b.Labels))
	found := false
	for _, l := range b.Labels {
		if l.ID == id {
			found = true
			continue
		}
		next = append(next, l)
	}
	if !found {
		return fmt.Errorf("remove label: label %q not found", id)
	}
	b.Labels = next
	return WriteBoard(b)
}
