package service

import (
	"fmt"

	bankan "github.com/thekondor/bankan"
)

// ListLanes returns all lanes for the board with id.
// For view boards it lists the view's own lanes; for regular boards it lists
// the board's lanes.
func (r *Registry) ListLanes(id string) ([]bankan.Lane, error) {
	var lanes []bankan.Lane
	err := r.withReadLock(id, func(dir string, isView bool) error {
		var err error
		lanes, err = bankan.ReadLanes(dir)
		return err
	})
	return lanes, err
}

// AddLane adds a new lane with name to the board with id.
// For view boards it calls AddViewLane (which enforces cross-board uniqueness).
func (r *Registry) AddLane(id, name string) (bankan.Lane, error) {
	var lane bankan.Lane
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			l, err := bankan.AddViewLane(vb, parent, name)
			if err != nil {
				return err
			}
			lane = l
			return nil
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		l, err := bankan.AddLane(b, name)
		if err != nil {
			return err
		}
		lane = l
		return nil
	})
	return lane, err
}

// RenameLane renames the lane from oldName to newName for the board with id.
// For view boards only the view's directory is affected; the parent is unchanged.
func (r *Registry) RenameLane(id, oldName, newName string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		// Both regular and view boards use the same RenameLane function;
		// for view boards we point a proxy board struct at the view directory.
		var b *bankan.Board
		if isView {
			b = &bankan.Board{}
			b.Dir = dir
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		return bankan.RenameLane(b, oldName, newName)
	})
}

// RemoveLane removes the empty lane with name from the board with id.
// For view boards it calls RemoveViewLane; for regular boards, RemoveLane.
func (r *Registry) RemoveLane(id, name string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			return bankan.RemoveViewLane(vb, name)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		return bankan.RemoveLane(b, name)
	})
}

// ReorderLanes reassigns lane order prefixes so they appear in the given name
// order. names must be a permutation of current lane names (case-insensitive).
func (r *Registry) ReorderLanes(id string, names []string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		var b *bankan.Board
		if isView {
			b = &bankan.Board{Dir: dir}
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		return bankan.ReorderLanes(b, names)
	})
}

// laneByName is a helper used in card operations.
func laneByName(lanes []bankan.Lane, name string) (bankan.Lane, error) {
	l, ok := bankan.LaneByName(lanes, name)
	if !ok {
		return bankan.Lane{}, &ErrNotFound{Resource: "lane", ID: name}
	}
	return l, nil
}

// requireLane reads lanes from dir and returns the one matching name.
func requireLane(dir, name string) (bankan.Lane, error) {
	lanes, err := bankan.ReadLanes(dir)
	if err != nil {
		return bankan.Lane{}, fmt.Errorf("read lanes: %w", err)
	}
	return laneByName(lanes, name)
}
