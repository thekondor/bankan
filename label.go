package bankan

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ArchivedLabelPrefix is prepended to a label name to mark it as archived.
// An archived label is hidden from pickers but not deleted.
const ArchivedLabelPrefix = "💼 "

// Label is a tag that can be assigned to cards within a board.
// Labels are unique per board by both ID and Name.
type Label struct {
	ID    string `yaml:"id"    json:"id"`
	Name  string `yaml:"name"  json:"name"`
	Color string `yaml:"color" json:"color"` // hex, e.g. "#ef4444"
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$`)

// ValidateLabel checks a single label for well-formedness.
func ValidateLabel(l Label) error {
	if strings.TrimSpace(l.ID) == "" {
		return errors.New("label id must not be empty")
	}
	if strings.TrimSpace(l.Name) == "" {
		return errors.New("label name must not be empty")
	}
	if !hexColorRe.MatchString(l.Color) {
		return fmt.Errorf("label color %q is not a valid hex color (e.g. \"#ef4444\")", l.Color)
	}
	return nil
}

// ValidateLabels checks all labels for well-formedness and uniqueness within
// the slice (both ID and Name must be unique).
func ValidateLabels(labels []Label) error {
	ids := make(map[string]struct{}, len(labels))
	names := make(map[string]struct{}, len(labels))

	for i, l := range labels {
		if err := ValidateLabel(l); err != nil {
			return fmt.Errorf("label[%d]: %w", i, err)
		}
		if _, dup := ids[l.ID]; dup {
			return fmt.Errorf("label id %q is not unique", l.ID)
		}
		ids[l.ID] = struct{}{}

		nameLower := strings.ToLower(l.Name)
		if _, dup := names[nameLower]; dup {
			return fmt.Errorf("label name %q is not unique (case-insensitive)", l.Name)
		}
		names[nameLower] = struct{}{}
	}
	return nil
}

// FindLabelByID returns the label with the given ID, or false if not found.
func FindLabelByID(labels []Label, id string) (Label, bool) {
	for _, l := range labels {
		if l.ID == id {
			return l, true
		}
	}
	return Label{}, false
}

// FindLabelByName returns the label with the given name (case-insensitive).
func FindLabelByName(labels []Label, name string) (Label, bool) {
	lower := strings.ToLower(name)
	for _, l := range labels {
		if strings.ToLower(l.Name) == lower {
			return l, true
		}
	}
	return Label{}, false
}

// IsLabelUsedInBoard reports whether labelID is referenced by any card in b
// (active lanes or archive), either as a regular label or as primary label.
func IsLabelUsedInBoard(b *Board, labelID string) (bool, error) {
	lanes, err := ReadLanes(b.Dir)
	if err != nil {
		return false, fmt.Errorf("is label used: read lanes: %w", err)
	}
	for _, lane := range lanes {
		cards, err := ListCards(lane)
		if err != nil {
			return false, fmt.Errorf("is label used: list cards in lane %q: %w", lane.Name, err)
		}
		for _, c := range cards {
			if c.PrimaryLabel == labelID {
				return true, nil
			}
			for _, lid := range c.Labels {
				if lid == labelID {
					return true, nil
				}
			}
		}
	}
	archived, err := ListArchivedCards(b)
	if err != nil {
		return false, fmt.Errorf("is label used: list archived cards: %w", err)
	}
	for _, c := range archived {
		if c.PrimaryLabel == labelID {
			return true, nil
		}
		for _, lid := range c.Labels {
			if lid == labelID {
				return true, nil
			}
		}
	}
	return false, nil
}
