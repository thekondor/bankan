package service

import (
	"fmt"

	bankan "github.com/thekondor/bankan"
)

// CardSearchResult holds the location of a card found during a cross-board search.
type CardSearchResult struct {
	BoardID   string
	BoardName string
	LaneName  string
	CardTitle string
}

// CardUpdate holds optional fields for UpdateCard.
// nil pointer fields mean "no change".
type CardUpdate struct {
	Title        *string
	Body         *string
	AddLabels    []string
	RemoveLabels []string
	// PrimaryLabel, when non-nil, sets the primary label (empty string clears it).
	// The primary label may overlap with the regular Labels slice; display layers
	// are responsible for deduplication.
	PrimaryLabel *string
}

// ListCards returns cards in the named lane for board id.
// For view boards the stubs are resolved against the parent.
func (r *Registry) ListCards(id, laneName string) ([]*bankan.Card, error) {
	var cards []*bankan.Card
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
			lane, err := requireLane(dir, laneName)
			if err != nil {
				return err
			}
			cs, err := bankan.ListViewCards(vb, parent, lane)
			if err != nil {
				return err
			}
			cards = cs
			return nil
		}
		lane, err := requireLane(dir, laneName)
		if err != nil {
			return err
		}
		cs, err := bankan.ListCards(lane)
		if err != nil {
			return err
		}
		cards = cs
		return nil
	})
	return cards, err
}

// ListAllCards returns a map of lane name → cards for all lanes in board id.
// For view boards cards are resolved from the parent.
func (r *Registry) ListAllCards(id string) (map[string][]*bankan.Card, error) {
	result := map[string][]*bankan.Card{}
	err := r.withReadLock(id, func(dir string, isView bool) error {
		lanes, err := bankan.ReadLanes(dir)
		if err != nil {
			return err
		}
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			for _, lane := range lanes {
				cs, err := bankan.ListViewCards(vb, parent, lane)
				if err != nil {
					return fmt.Errorf("list view cards for lane %q: %w", lane.Name, err)
				}
				result[lane.Name] = cs
			}
			return nil
		}
		for _, lane := range lanes {
			cs, err := bankan.ListCards(lane)
			if err != nil {
				return fmt.Errorf("list cards for lane %q: %w", lane.Name, err)
			}
			result[lane.Name] = cs
		}
		return nil
	})
	return result, err
}

// ListArchivedCards returns all archived cards for a regular board.
// Returns ErrForbidden for view boards (view boards don't use _archive).
func (r *Registry) ListArchivedCards(id string) ([]*bankan.Card, error) {
	var cards []*bankan.Card
	err := r.withReadLock(id, func(dir string, isView bool) error {
		if isView {
			return &ErrForbidden{Reason: "view boards do not archive cards"}
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		cs, err := bankan.ListArchivedCards(b)
		if err != nil {
			return err
		}
		cards = cs
		return nil
	})
	return cards, err
}

// GetCard returns the card with cardID from board id.
// Searches lanes and (for regular boards) the archive.
// For view boards the card is resolved from the parent.
func (r *Registry) GetCard(id, cardID string) (*bankan.Card, error) {
	var card *bankan.Card
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
			stub, viewLane, err := bankan.FindViewCardStub(vb, cardID)
			if err != nil {
				return &ErrNotFound{Resource: "card", ID: cardID}
			}
			c, err := bankan.ResolveViewCard(stub, parent)
			if err != nil {
				return err
			}
			c.Lane = viewLane.Name
			card = c
			return nil
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, true)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		card = c
		return nil
	})
	return card, err
}

// AddCard creates a new card in laneName for board id.
// For view boards the card is created in the parent (with FilterLabel) and a stub placed in the view lane.
func (r *Registry) AddCard(id, laneName, title, body string, labelIDs []string) (*bankan.Card, error) {
	var card *bankan.Card
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
			viewLane, err := requireLane(dir, laneName)
			if err != nil {
				return err
			}
			c, _, err := bankan.AddViewCard(vb, parent, viewLane, title, body, labelIDs)
			if err != nil {
				return err
			}
			card = c
			return nil
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		lane, err := requireLane(dir, laneName)
		if err != nil {
			return err
		}
		// Validate label IDs against board labels.
		for _, lid := range labelIDs {
			if _, ok := bankan.FindLabelByID(b.Labels, lid); !ok {
				return &ErrNotFound{Resource: "label", ID: lid}
			}
		}
		c, err := bankan.AddCard(b, lane, title, body, labelIDs)
		if err != nil {
			return err
		}
		card = c
		return nil
	})
	return card, err
}

// UpdateCard applies the given update to card cardID in board id.
// For view boards, edits are forwarded to the parent; removing the filter label is blocked.
func (r *Registry) UpdateCard(id, cardID string, update CardUpdate) (*bankan.Card, error) {
	var card *bankan.Card
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
			// Verify card is tracked by this view.
			if _, _, err := bankan.FindViewCardStub(vb, cardID); err != nil {
				return &ErrNotFound{Resource: "card", ID: cardID}
			}
			// Block removal of the filter label.
			for _, lid := range update.RemoveLabels {
				if lid == vb.FilterLabel {
					return &ErrForbidden{Reason: fmt.Sprintf("cannot remove filter label %q via view board edit", lid)}
				}
			}
			b = parent
		} else {
			var err error
			b, err = bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
		}
		c, err := bankan.FindCard(b, cardID, false)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		changed := false
		if update.Title != nil {
			c.Title = *update.Title
			changed = true
		}
		if update.Body != nil {
			c.Body = *update.Body
			changed = true
		}
		for _, lid := range update.AddLabels {
			if _, ok := bankan.FindLabelByID(b.Labels, lid); !ok {
				return &ErrNotFound{Resource: "label", ID: lid}
			}
			if !containsStr(c.Labels, lid) {
				c.Labels = append(c.Labels, lid)
				changed = true
			}
		}
		for _, lid := range update.RemoveLabels {
			next := make([]string, 0, len(c.Labels))
			for _, l := range c.Labels {
				if l != lid {
					next = append(next, l)
				}
			}
			if len(next) != len(c.Labels) {
				c.Labels = next
				changed = true
			}
		}
		if update.PrimaryLabel != nil {
			newPrimary := *update.PrimaryLabel
			if newPrimary != "" {
				if _, ok := bankan.FindLabelByID(b.Labels, newPrimary); !ok {
					return &ErrNotFound{Resource: "label", ID: newPrimary}
				}
				if !containsStr(c.Labels, newPrimary) {
					c.Labels = append(c.Labels, newPrimary)
				}
			}
			c.PrimaryLabel = newPrimary
			changed = true
		}
		if !changed {
			card = c
			return nil
		}
		if err := bankan.WriteCard(c); err != nil {
			return err
		}
		card = c
		return nil
	})
	return card, err
}

// MoveCard moves card cardID to toLaneName in board id.
func (r *Registry) MoveCard(id, cardID, toLaneName string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			toLane, err := requireLane(dir, toLaneName)
			if err != nil {
				return err
			}
			return bankan.MoveViewCard(vb, parent, cardID, toLane)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, false)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		toLane, err := requireLane(dir, toLaneName)
		if err != nil {
			return err
		}
		return bankan.MoveCard(b, c, toLane)
	})
}

// ReorderCard changes the within-lane position of a card.
// For view boards: reorders the stub and propagates to the parent's labelled subset.
// For regular boards: reorders among all cards in the lane.
func (r *Registry) ReorderCard(id, cardID string, newIndex int) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			return bankan.ReorderViewCard(vb, parent, cardID, newIndex)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, false)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		lane, err := requireLane(dir, c.Lane)
		if err != nil {
			return err
		}
		return bankan.ReorderCard(lane, cardID, newIndex)
	})
}

// ArchiveCard archives card cardID in board id.
// For view boards this removes the filter label (RemoveCardFromView).
func (r *Registry) ArchiveCard(id, cardID string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			return bankan.RemoveCardFromView(vb, parent, cardID)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, false)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		return bankan.ArchiveCard(b, c)
	})
}

// RestoreCard restores archived card cardID to toLaneName in regular board id.
// Returns ErrForbidden for view boards.
func (r *Registry) RestoreCard(id, cardID, toLaneName string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			return &ErrForbidden{Reason: "restore is not available in view boards; operate on the parent board"}
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, true)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		toLane, err := requireLane(dir, toLaneName)
		if err != nil {
			return err
		}
		return bankan.RestoreCard(b, c, toLane)
	})
}

// DeleteCard permanently deletes card cardID from board id.
// For view boards this removes the filter label (same as ArchiveCard).
func (r *Registry) DeleteCard(id, cardID string) error {
	return r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			vb, err := bankan.ReadViewBoard(dir)
			if err != nil {
				return err
			}
			parent, err := bankan.ParentBoard(vb)
			if err != nil {
				return err
			}
			return bankan.RemoveCardFromView(vb, parent, cardID)
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		c, err := bankan.FindCard(b, cardID, true)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		return bankan.DeleteCard(c)
	})
}

// DuplicateCard duplicates card cardID in regular board id, placing the copy in
// the same lane. Returns ErrForbidden for view boards.
func (r *Registry) DuplicateCard(id, cardID string) (*bankan.Card, error) {
	var card *bankan.Card
	err := r.withWriteLock(id, func(dir string, isView bool) error {
		if isView {
			return &ErrForbidden{Reason: "duplicate card is not supported on view boards; operate on the parent board"}
		}
		b, err := bankan.ReadBoard(dir)
		if err != nil {
			return err
		}
		source, err := bankan.FindCard(b, cardID, false)
		if err != nil {
			return &ErrNotFound{Resource: "card", ID: cardID}
		}
		c, err := bankan.DuplicateCard(b, source)
		if err != nil {
			return err
		}
		card = c
		return nil
	})
	return card, err
}

// SearchCard searches for cardID across all registered boards (active lanes only, no archive).
// Archived view boards are skipped unless includeArchived is true.
// Returns all matches; a card visible in both a regular board and a view board yields two results.
func (r *Registry) SearchCard(cardID string, includeArchived bool) ([]CardSearchResult, error) {
	boardIDs := r.BoardIDs()
	var results []CardSearchResult
	for _, id := range boardIDs {
		var result *CardSearchResult
		err := r.withReadLock(id, func(dir string, isView bool) error {
			if isView {
				vb, err := bankan.ReadViewBoard(dir)
				if err != nil {
					return err
				}
				if !includeArchived && vb.ArchivedAt != nil {
					return nil // skip archived view board
				}
				parent, err := bankan.ParentBoard(vb)
				if err != nil {
					return err
				}
				stub, viewLane, err := bankan.FindViewCardStub(vb, cardID)
				if err != nil {
					return nil // not in this view board
				}
				c, err := bankan.ResolveViewCard(stub, parent)
				if err != nil {
					return err
				}
				result = &CardSearchResult{
					BoardID:   id,
					BoardName: vb.Name,
					LaneName:  viewLane.Name,
					CardTitle: c.Title,
				}
				return nil
			}
			b, err := bankan.ReadBoard(dir)
			if err != nil {
				return err
			}
			c, err := bankan.FindCard(b, cardID, false) // active only, no archive
			if err != nil {
				return nil // not in this board
			}
			result = &CardSearchResult{
				BoardID:   id,
				BoardName: b.Name,
				LaneName:  c.Lane,
				CardTitle: c.Title,
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("search card %q in board %q: %w", cardID, id, err)
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	return results, nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
