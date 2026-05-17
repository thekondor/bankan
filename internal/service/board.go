package service

import (
	"fmt"
	"path/filepath"

	bankan "github.com/thekondor/bankan"
)

// GetBoard returns the Board for a regular board ID.
// Returns ErrNotFound if the board does not exist or is a view board.
func (r *Registry) GetBoard(id string) (*bankan.Board, error) {
	var board *bankan.Board
	err := r.withReadLock(id, func(dir string, isView bool) error {
		if isView {
			return &ErrNotFound{Resource: "regular board", ID: id}
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return fmt.Errorf("read board %q: %w", id, err)
		}
		board = b
		return nil
	})
	return board, err
}

// GetViewBoard returns the ViewBoard and its parent Board for a view board ID.
// Returns ErrNotFound if the ID is a regular board.
func (r *Registry) GetViewBoard(id string) (*bankan.ViewBoard, *bankan.Board, error) {
	var vb *bankan.ViewBoard
	var parent *bankan.Board
	err := r.withReadLock(id, func(dir string, isView bool) error {
		if !isView {
			return &ErrNotFound{Resource: "view board", ID: id}
		}
		var err error
		vb, err = bankan.ReadViewBoard(dir)
		if err != nil {
			return fmt.Errorf("read view board %q: %w", id, err)
		}
		parent, err = bankan.ParentBoard(vb)
		if err != nil {
			return fmt.Errorf("load parent board for view %q: %w", id, err)
		}
		return nil
	})
	return vb, parent, err
}

// InitBoard creates a new board with the given name inside r.RootDir().
// The directory name is derived by slugifying the display name so that
// "My Project" becomes rootDir/my-project.
// Returns ErrForbidden when no rootDir is configured.
func (r *Registry) InitBoard(name string) (*bankan.Board, error) {
	if r.rootDir == "" {
		return nil, &ErrForbidden{Reason: "board creation requires a root directory (start bankan serve with a container directory)"}
	}
	dirName := bankan.Slugify(name)
	if dirName == "" {
		return nil, &ErrBadRequest{Reason: "name produces an empty slug; use letters, digits, or hyphens"}
	}
	dir := filepath.Join(r.rootDir, dirName)
	order := r.nextBoardOrder()
	b, err := bankan.InitBoard(dir, name)
	if err != nil {
		return nil, fmt.Errorf("init board %q: %w", name, err)
	}
	b.Order = order
	if err := bankan.WriteBoard(b); err != nil {
		return nil, fmt.Errorf("init board %q: write order: %w", name, err)
	}
	// Register the new board in the registry.
	if _, err := r.Register(dir); err != nil {
		return nil, fmt.Errorf("register board after init: %w", err)
	}
	return b, nil
}

// InitViewBoard creates a new view board with the given name inside r.RootDir(),
// filtered to cards on parentID that carry filterLabelID.
// The directory name is derived by slugifying the display name.
// Returns ErrForbidden when no rootDir is configured or parentID is itself a view board.
func (r *Registry) InitViewBoard(name, parentID, filterLabelID string) (*bankan.ViewBoard, error) {
	if r.rootDir == "" {
		return nil, &ErrForbidden{Reason: "view board creation requires a root directory (start bankan serve with a container directory)"}
	}
	dirName := bankan.Slugify(name)
	if dirName == "" {
		return nil, &ErrBadRequest{Reason: "name produces an empty slug; use letters, digits, or hyphens"}
	}
	parentEntry, err := r.getEntry(parentID)
	if err != nil {
		return nil, fmt.Errorf("find parent board: %w", err)
	}
	if parentEntry.isView {
		return nil, &ErrForbidden{Reason: "parent board must be a regular board, not a view board"}
	}
	dir := filepath.Join(r.rootDir, dirName)
	order := r.nextBoardOrder()
	vb, err := bankan.InitViewBoard(dir, name, parentEntry.dir, filterLabelID)
	if err != nil {
		return nil, fmt.Errorf("init view board %q: %w", name, err)
	}
	vb.Order = order
	if err := bankan.WriteViewBoard(vb); err != nil {
		return nil, fmt.Errorf("init view board %q: write order: %w", name, err)
	}
	if _, err := r.Register(dir); err != nil {
		return nil, fmt.Errorf("register view board after init: %w", err)
	}
	parent, err := bankan.ReadBoard(parentEntry.dir)
	if err != nil {
		return nil, fmt.Errorf("load parent for initial sync: %w", err)
	}
	if err := bankan.SyncViewBoard(vb, parent); err != nil {
		return nil, fmt.Errorf("initial sync of view board %q: %w", name, err)
	}
	return vb, nil
}

// UpdateBoardColor sets the primary accent color on a regular or view board.
func (r *Registry) UpdateBoardColor(id, color string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return fmt.Errorf("read view board %q: %w", id, err)
			}
			return bankan.UpdateViewBoardColor(vb, color)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return fmt.Errorf("read board %q: %w", id, err)
		}
		return bankan.UpdateBoardColor(b, color)
	})
}

// SyncViewBoard syncs a view board with its parent (adds missing stubs,
// removes orphaned stubs). Only valid for view board IDs.
func (r *Registry) SyncViewBoard(id string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if !isView {
			return &ErrForbidden{Reason: fmt.Sprintf("board %q is not a view board", id)}
		}
		vb, err := bankan.ReadViewBoard(dir)
		if err != nil {
			return err
		}
		parent, err := bankan.ParentBoard(vb)
		if err != nil {
			return err
		}
		return bankan.SyncViewBoard(vb, parent)
	})
}

// HideBoard sets hidden:true on a regular or view board.
func (r *Registry) HideBoard(id string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return fmt.Errorf("read view board %q: %w", id, err)
			}
			return bankan.HideViewBoard(vb)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return fmt.Errorf("read board %q: %w", id, err)
		}
		return bankan.HideBoard(b)
	})
}

// ShowBoard clears hidden on a regular or view board.
func (r *Registry) ShowBoard(id string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return fmt.Errorf("read view board %q: %w", id, err)
			}
			return bankan.ShowViewBoard(vb)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return fmt.Errorf("read board %q: %w", id, err)
		}
		return bankan.ShowBoard(b)
	})
}

// IsHiddenBoard reports whether the board with id has hidden:true.
func (r *Registry) IsHiddenBoard(id string) bool {
	hidden := false
	_ = r.withReadLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return nil
			}
			hidden = vb.Hidden
		} else {
			b, err := bankan.ReadBoard(dir)
			if err != nil {
				return nil
			}
			hidden = b.Hidden
		}
		return nil
	})
	return hidden
}

// ArchiveViewBoard archives a view board (sets archived_at in view.md).
// Only valid for view board IDs.
// When archiveLabel is true, the filter label on the parent board is renamed
// by prepending "💼 " to its name, marking it as an archived filter label.
func (r *Registry) ArchiveViewBoard(id string, archiveLabel bool) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if !isView {
			return &ErrForbidden{Reason: fmt.Sprintf("board %q is not a view board", id)}
		}
		vb, err := bankan.ReadViewBoard(dir)
		if err != nil {
			return err
		}
		if err := bankan.ArchiveViewBoard(vb); err != nil {
			return err
		}
		if archiveLabel {
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return fmt.Errorf("load parent board to archive filter label: %w", err)
			}
			lbl, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel)
			if !ok {
				// Label already removed from parent — not an error.
				return nil
			}
			lbl.Name = "💼 " + lbl.Name
			if err := bankan.UpdateLabel(parent, lbl); err != nil {
				return fmt.Errorf("rename filter label: %w", err)
			}
		}
		return nil
	})
}
