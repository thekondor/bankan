package service

import (
	"fmt"

	bankan "github.com/thekondor/bankan"
)

// LabelUpdate holds optional fields for UpdateLabel.
type LabelUpdate struct {
	Name  *string
	Color *string
}

// ListLabels returns all labels for board id.
// For view boards the parent board's labels are returned.
func (r *Registry) ListLabels(id string) ([]bankan.Label, error) {
	var labels []bankan.Label
	err := r.withReadLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			labels = parent.Labels
			return nil
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		labels = b.Labels
		return nil
	})
	return labels, err
}

// AddLabel adds a new label with name and color to board id.
// Returns ErrForbidden for view boards.
func (r *Registry) AddLabel(id, name, color string) (bankan.Label, error) {
	var label bankan.Label
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			return &ErrForbidden{Reason: "labels cannot be added to a view board; add to the parent board"}
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		existing := make(map[string]struct{}, len(b.Labels))
		for _, l := range b.Labels {
			existing[l.ID] = struct{}{}
		}
		l := bankan.Label{
			ID:    bankan.NewUniqueID(existing),
			Name:  name,
			Color: color,
		}
		if err := bankan.AddLabel(b, l); err != nil {
			return fmt.Errorf("add label: %w", err)
		}
		label = l
		return nil
	})
	return label, err
}

// UpdateLabel updates the name and/or color of label labelID in board id.
// For view boards the edit is forwarded to the parent board.
func (r *Registry) UpdateLabel(id, labelID string, update LabelUpdate) (bankan.Label, error) {
	var label bankan.Label
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		var b *bankan.Board
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			b = parent
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		l, ok := bankan.FindLabelByID(b.Labels, labelID)
		if !ok {
			return &ErrNotFound{Resource: "label", ID: labelID}
		}
		if update.Name != nil {
			l.Name = *update.Name
		}
		if update.Color != nil {
			l.Color = *update.Color
		}
		if err := bankan.UpdateLabel(b, l); err != nil {
			return fmt.Errorf("update label: %w", err)
		}
		label = l
		return nil
	})
	return label, err
}

// IsLabelUsed reports whether labelID is referenced by any card in the board.
// For view boards the parent board's cards are checked.
func (r *Registry) IsLabelUsed(id, labelID string) (bool, error) {
	var used bool
	err := r.withReadLock(id, func(dir string, isView bool) error {
		var b *bankan.Board
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			b = parent
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		var err error
		used, err = bankan.IsLabelUsedInBoard(b, labelID)
		return err
	})
	return used, err
}

// ArchiveLabel prefixes label labelID's name with ArchivedLabelPrefix.
// For view boards the edit is forwarded to the parent board.
func (r *Registry) ArchiveLabel(id, labelID string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		var b *bankan.Board
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			b = parent
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		lbl, ok := bankan.FindLabelByID(b.Labels, labelID)
		if !ok {
			return &ErrNotFound{Resource: "label", ID: labelID}
		}
		lbl.Name = bankan.ArchivedLabelPrefix + lbl.Name
		if err := bankan.UpdateLabel(b, lbl); err != nil {
			return fmt.Errorf("archive label: %w", err)
		}
		return nil
	})
}

// RemoveLabel removes or archives label labelID from board id.
// When force is false (default) the label is archived (prefixed with ArchivedLabelPrefix)
// so it remains visible on existing cards but is excluded from pickers.
// When force is true the label is permanently deleted.
// For view boards, force-deletion of the filter label is always blocked.
func (r *Registry) RemoveLabel(id, labelID string, force bool) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		var b *bankan.Board
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			if force && labelID == vb.FilterLabel {
				return &ErrForbidden{Reason: fmt.Sprintf("cannot remove filter label %q: it is the founding label of view board %q", labelID, vb.Name)}
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			b = parent
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		if !force {
			lbl, ok := bankan.FindLabelByID(b.Labels, labelID)
			if !ok {
				return &ErrNotFound{Resource: "label", ID: labelID}
			}
			lbl.Name = bankan.ArchivedLabelPrefix + lbl.Name
			if err := bankan.UpdateLabel(b, lbl); err != nil {
				return fmt.Errorf("archive label: %w", err)
			}
			return nil
		}
		return bankan.RemoveLabel(b, labelID)
	})
}
