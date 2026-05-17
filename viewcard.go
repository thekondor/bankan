package bankan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ViewCardStub is a thin reference file stored in a view board's lane directory.
// Its filename follows the same NNN-<id>-<slug>.md pattern as a regular card.
// The actual card data always lives in the parent board.
//
// Stub file format (YAML frontmatter only):
//
//	---
//	card_id: ab12c
//	---
type ViewCardStub struct {
	CardID string `yaml:"card_id"`

	// Runtime-only fields.
	FilePath string `yaml:"-"` // absolute path to the stub file in the view board
	Lane     string `yaml:"-"` // view lane display name
}

// readViewCardStub reads and parses a stub file.
func readViewCardStub(filePath string) (*ViewCardStub, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read view card stub: %w", err)
	}
	var s ViewCardStub
	_, err = Parse(data, &s)
	if err != nil {
		return nil, fmt.Errorf("read view card stub %s: %w", filePath, err)
	}
	s.FilePath = filePath
	return &s, nil
}

// writeViewCardStub writes a stub file to laneDir with the given card metadata.
// The slug is derived from the card title and frozen at creation (same invariant
// as regular card slugs).
func writeViewCardStub(laneDir, cardID, slug string, order int) (*ViewCardStub, error) {
	s := &ViewCardStub{CardID: cardID}
	data, err := Serialize(s, "")
	if err != nil {
		return nil, fmt.Errorf("write view card stub: serialize: %w", err)
	}
	fname := cardFilename(order, cardID, slug)
	fpath := filepath.Join(laneDir, fname)
	if err := os.WriteFile(fpath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write view card stub: %w", err)
	}
	s.FilePath = fpath
	return s, nil
}

// ResolveViewCard fetches the actual Card from the parent board for the given stub.
func ResolveViewCard(stub *ViewCardStub, parent *Board) (*Card, error) {
	c, err := FindCard(parent, stub.CardID, true)
	if err != nil {
		return nil, fmt.Errorf("resolve view card %q: %w", stub.CardID, err)
	}
	return c, nil
}

// ListViewCardStubs returns all stubs in a view lane, sorted by order prefix.
func ListViewCardStubs(lane Lane) ([]*ViewCardStub, error) {
	entries, err := os.ReadDir(lane.Dir)
	if err != nil {
		return nil, fmt.Errorf("list view card stubs: %w", err)
	}

	type entry struct {
		order int
		path  string
	}
	var files []entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		order, _, _, ok := parseCardFilename(e.Name())
		if !ok {
			continue
		}
		files = append(files, entry{order, filepath.Join(lane.Dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].order < files[j].order })

	stubs := make([]*ViewCardStub, 0, len(files))
	for _, f := range files {
		s, err := readViewCardStub(f.path)
		if err != nil {
			return nil, err
		}
		s.Lane = lane.Name
		stubs = append(stubs, s)
	}
	return stubs, nil
}

// ListViewCards returns the resolved Cards (from the parent board) for all stubs
// in a view lane, preserving stub order.
func ListViewCards(vb *ViewBoard, parent *Board, lane Lane) ([]*Card, error) {
	stubs, err := ListViewCardStubs(lane)
	if err != nil {
		return nil, err
	}
	cards := make([]*Card, 0, len(stubs))
	for _, s := range stubs {
		c, err := ResolveViewCard(s, parent)
		if err != nil {
			return nil, err
		}
		// Expose the view-lane name rather than the parent's lane name.
		c.Lane = lane.Name
		cards = append(cards, c)
	}
	return cards, nil
}

// FindViewCardStub scans all lanes of the view board for a stub with the given
// card ID. Returns the stub and its lane, or an error if not found.
func FindViewCardStub(vb *ViewBoard, cardID string) (*ViewCardStub, Lane, error) {
	lanes, err := ReadLanes(vb.Dir)
	if err != nil {
		return nil, Lane{}, err
	}
	for _, lane := range lanes {
		stubs, err := ListViewCardStubs(lane)
		if err != nil {
			return nil, Lane{}, err
		}
		for _, s := range stubs {
			if s.CardID == cardID {
				s.Lane = lane.Name
				return s, lane, nil
			}
		}
	}
	return nil, Lane{}, fmt.Errorf("view card stub %q not found", cardID)
}

// collectViewStubIDs returns a set of all card IDs currently tracked in the view.
func collectViewStubIDs(vb *ViewBoard) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	lanes, err := ReadLanes(vb.Dir)
	if err != nil {
		return nil, err
	}
	for _, lane := range lanes {
		stubs, err := ListViewCardStubs(lane)
		if err != nil {
			return nil, err
		}
		for _, s := range stubs {
			ids[s.CardID] = struct{}{}
		}
	}
	return ids, nil
}

// SyncViewBoard synchronises the view board with its parent:
//
//  1. Scans the parent board (all lanes and archive) for cards that carry the
//     view's FilterLabel.
//  2. For each such card not yet tracked in the view, creates a stub in the
//     matching view lane (by lane name). If the card's parent lane doesn't exist
//     in the view, the stub is placed in the first view lane; if there are no
//     view lanes, it is skipped (a warning-level situation callers may handle).
//  3. Relocates existing stubs whose parent card has moved to a different lane.
//     If the parent's new lane has a matching view lane, the stub is moved there.
//     If there is no matching view lane, the stub stays where it is.
//  4. Removes stubs whose card no longer carries the FilterLabel (orphan cleanup).
func SyncViewBoard(vb *ViewBoard, parent *Board) error {
	// Build set of parent cards that carry the filter label.
	parentCards, err := allParentCardsWithLabel(parent, vb.FilterLabel)
	if err != nil {
		return fmt.Errorf("sync view board: %w", err)
	}
	wantIDs := make(map[string]struct{}, len(parentCards))
	for _, c := range parentCards {
		wantIDs[c.ID] = struct{}{}
	}

	// Index parent cards by ID for fast lane lookups during stub relocation.
	parentByID := make(map[string]*Card, len(parentCards))
	for _, c := range parentCards {
		parentByID[c.ID] = c
	}

	// Build current stub state.
	haveIDs, err := collectViewStubIDs(vb)
	if err != nil {
		return fmt.Errorf("sync view board: %w", err)
	}

	viewLanes, err := ReadLanes(vb.Dir)
	if err != nil {
		return fmt.Errorf("sync view board: %w", err)
	}

	// Add missing stubs.
	for _, c := range parentCards {
		if _, exists := haveIDs[c.ID]; exists {
			continue
		}
		targetLane, ok := LaneByName(viewLanes, c.Lane)
		if !ok {
			// Parent lane not in view: use first view lane as fallback.
			if len(viewLanes) == 0 {
				continue
			}
			targetLane = viewLanes[0]
		}
		order, err := nextOrderInLane(targetLane.Dir)
		if err != nil {
			return fmt.Errorf("sync view board: next order for lane %q: %w", targetLane.Name, err)
		}
		slug := slugify(c.Title)
		if slug == "" {
			slug = c.ID
		}
		if _, err := writeViewCardStub(targetLane.Dir, c.ID, slug, order); err != nil {
			return fmt.Errorf("sync view board: create stub for card %q: %w", c.ID, err)
		}
	}

	// Relocate stubs whose parent card has moved to a different lane.
	// Only wanted stubs are considered; orphans are handled by the removal pass.
	// If the parent's current lane has no matching view lane, the stub stays put.
	for _, lane := range viewLanes {
		stubs, err := ListViewCardStubs(lane)
		if err != nil {
			return fmt.Errorf("sync view board: list stubs in lane %q: %w", lane.Name, err)
		}
		for _, s := range stubs {
			if _, want := wantIDs[s.CardID]; !want {
				continue
			}
			parentCard := parentByID[s.CardID]
			targetLane, ok := LaneByName(viewLanes, parentCard.Lane)
			if !ok {
				continue // parent lane not mirrored in view — leave stub where it is
			}
			if s.Lane == targetLane.Name {
				continue // already in the right lane
			}
			if err := moveStub(s, targetLane); err != nil {
				return fmt.Errorf("sync view board: relocate stub for card %q: %w", s.CardID, err)
			}
		}
	}

	// Remove orphaned stubs (card no longer has the filter label).
	for _, lane := range viewLanes {
		stubs, err := ListViewCardStubs(lane)
		if err != nil {
			return fmt.Errorf("sync view board: list stubs in lane %q: %w", lane.Name, err)
		}
		for _, s := range stubs {
			if _, want := wantIDs[s.CardID]; want {
				continue
			}
			if err := os.Remove(s.FilePath); err != nil {
				return fmt.Errorf("sync view board: remove orphan stub %q: %w", s.CardID, err)
			}
		}
	}

	return nil
}

// allParentCardsWithLabel returns all cards in the parent board (lanes + archive)
// that have labelID in their Labels slice.
func allParentCardsWithLabel(parent *Board, labelID string) ([]*Card, error) {
	var result []*Card

	lanes, err := ReadLanes(parent.Dir)
	if err != nil {
		return nil, err
	}
	for _, lane := range lanes {
		cards, err := ListCards(lane)
		if err != nil {
			return nil, err
		}
		for _, c := range cards {
			if containsLabelID(c.Labels, labelID) {
				result = append(result, c)
			}
		}
	}

	archived, err := ListArchivedCards(parent)
	if err != nil {
		return nil, err
	}
	for _, c := range archived {
		if containsLabelID(c.Labels, labelID) {
			result = append(result, c)
		}
	}

	return result, nil
}

// containsLabelID reports whether a label ID slice contains id.
func containsLabelID(labels []string, id string) bool {
	for _, l := range labels {
		if l == id {
			return true
		}
	}
	return false
}

// AddViewCard creates a new card in the parent board (with the view's FilterLabel
// automatically applied) and places a stub in the corresponding view lane.
//
// If the target view lane exists in the parent board (by name), the card is
// created there. Otherwise, the card is placed in the first parent lane. If
// there are no parent lanes, an error is returned.
//
// The card's title and body are passed through. Additional label IDs (beyond
// the filter label) are validated against the parent board.
func AddViewCard(vb *ViewBoard, parent *Board, viewLane Lane, title, body string, extraLabelIDs []string) (*Card, *ViewCardStub, error) {
	// Determine the parent lane to place the card.
	parentLanes, err := ReadLanes(parent.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("add view card: %w", err)
	}
	var targetParentLane Lane
	if pl, ok := LaneByName(parentLanes, viewLane.Name); ok {
		targetParentLane = pl
	} else {
		if len(parentLanes) == 0 {
			return nil, nil, errors.New("add view card: parent board has no lanes")
		}
		targetParentLane = parentLanes[0]
	}

	// Build the label list: filter label + any extras (deduped).
	labelIDs := []string{vb.FilterLabel}
	for _, id := range extraLabelIDs {
		if id != vb.FilterLabel {
			labelIDs = append(labelIDs, id)
		}
	}

	// Create card in parent.
	c, err := AddCard(parent, targetParentLane, title, body, labelIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("add view card: %w", err)
	}

	// Create stub in view lane.
	order, err := nextOrderInLane(viewLane.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("add view card: next order in view lane: %w", err)
	}
	slug := slugify(title)
	if slug == "" {
		slug = c.ID
	}
	stub, err := writeViewCardStub(viewLane.Dir, c.ID, slug, order)
	if err != nil {
		return nil, nil, fmt.Errorf("add view card: write stub: %w", err)
	}
	stub.Lane = viewLane.Name

	return c, stub, nil
}

// MoveViewCard moves a card within the view board.
//
// If toLane exists (by name) in the parent board, the card's actual file is
// also moved in the parent (MoveCard). The stub is updated to point to the
// new view lane.
//
// If toLane exists only in the view board (not in the parent), the card file
// stays in the parent unchanged and only the stub is moved.
func MoveViewCard(vb *ViewBoard, parent *Board, cardID string, toLane Lane) error {
	stub, fromLane, err := FindViewCardStub(vb, cardID)
	if err != nil {
		return fmt.Errorf("move view card: %w", err)
	}
	if strings.EqualFold(fromLane.Name, toLane.Name) {
		return nil // nothing to do
	}

	// Determine whether toLane exists in the parent.
	parentLanes, err := ReadLanes(parent.Dir)
	if err != nil {
		return fmt.Errorf("move view card: %w", err)
	}
	if parentLane, ok := LaneByName(parentLanes, toLane.Name); ok {
		// Move in parent too.
		c, err := FindCard(parent, cardID, false)
		if err != nil {
			return fmt.Errorf("move view card: find in parent: %w", err)
		}
		if err := MoveCard(parent, c, parentLane); err != nil {
			return fmt.Errorf("move view card: move in parent: %w", err)
		}
	}
	// In both cases: move the stub in the view.
	return moveStub(stub, toLane)
}

// moveStub moves a stub file from its current lane to toLane, assigning the
// next available order prefix in the destination.
func moveStub(stub *ViewCardStub, toLane Lane) error {
	srcBase := filepath.Base(stub.FilePath)
	_, cardID, slug, ok := parseCardFilename(srcBase)
	if !ok {
		return fmt.Errorf("move stub: unexpected filename %q", srcBase)
	}

	order, err := nextOrderInLane(toLane.Dir)
	if err != nil {
		return fmt.Errorf("move stub: %w", err)
	}

	newBase := cardFilename(order, cardID, slug)
	newPath := filepath.Join(toLane.Dir, newBase)

	newStub := &ViewCardStub{CardID: stub.CardID}
	data, err := Serialize(newStub, "")
	if err != nil {
		return fmt.Errorf("move stub: serialize: %w", err)
	}
	if err := os.WriteFile(newPath, data, 0o644); err != nil {
		return fmt.Errorf("move stub: write new: %w", err)
	}
	if err := os.Remove(stub.FilePath); err != nil {
		return fmt.Errorf("move stub: remove old: %w", err)
	}

	stub.FilePath = newPath
	stub.Lane = toLane.Name
	return nil
}

// reorderStubsInLane reorders stubs within a view lane by redistributing the
// existing set of order prefixes to match the given slice order, using a
// two-phase temp-rename to avoid collisions.
func reorderStubsInLane(lane Lane, stubs []*ViewCardStub, oldIndex, newIndex int) error {
	prefixes := make([]int, len(stubs))
	for i, s := range stubs {
		o, _, _, _ := parseCardFilename(filepath.Base(s.FilePath))
		prefixes[i] = o
	}
	sort.Ints(prefixes)

	moved := stubs[oldIndex]
	newOrder := make([]*ViewCardStub, 0, len(stubs))
	for i, s := range stubs {
		if i == oldIndex {
			continue
		}
		newOrder = append(newOrder, s)
	}
	newOrder = append(newOrder[:newIndex:newIndex], append([]*ViewCardStub{moved}, newOrder[newIndex:]...)...)

	type stubRename struct {
		from string
		to   string
	}
	var renames []stubRename
	for i, s := range newOrder {
		wantOrder := prefixes[i]
		curOrder, id, slug, _ := parseCardFilename(filepath.Base(s.FilePath))
		if curOrder == wantOrder {
			continue
		}
		renames = append(renames, stubRename{
			from: s.FilePath,
			to:   filepath.Join(lane.Dir, cardFilename(wantOrder, id, slug)),
		})
	}

	type temp struct {
		tmp string
		dst string
	}
	temps := make([]temp, 0, len(renames))
	for i, r := range renames {
		tmp := filepath.Join(lane.Dir, fmt.Sprintf("_reorder_stub_%d_", i))
		if err := os.Rename(r.from, tmp); err != nil {
			return fmt.Errorf("reorder stubs: temp rename: %w", err)
		}
		temps = append(temps, temp{tmp, r.to})
	}
	for _, t := range temps {
		if err := os.Rename(t.tmp, t.dst); err != nil {
			return fmt.Errorf("reorder stubs: final rename: %w", err)
		}
	}
	return nil
}

// ReorderViewCard changes the position of a card stub within its current view
// lane (newIndex is 0-based) and propagates the ordering to the parent board
// among cards sharing vb.FilterLabel in the same lane.
func ReorderViewCard(vb *ViewBoard, parent *Board, cardID string, newIndex int) error {
	stub, lane, err := FindViewCardStub(vb, cardID)
	if err != nil {
		return fmt.Errorf("reorder view card: %w", err)
	}

	stubs, err := ListViewCardStubs(lane)
	if err != nil {
		return fmt.Errorf("reorder view card: %w", err)
	}

	if newIndex < 0 || newIndex >= len(stubs) {
		return fmt.Errorf("reorder view card: index %d out of range [0, %d)", newIndex, len(stubs))
	}

	oldIndex := -1
	for i, s := range stubs {
		if s.CardID == stub.CardID {
			oldIndex = i
			break
		}
	}
	if oldIndex == -1 {
		return fmt.Errorf("reorder view card: stub for %q not found", cardID)
	}
	if oldIndex == newIndex {
		return nil
	}

	if err := reorderStubsInLane(lane, stubs, oldIndex, newIndex); err != nil {
		return fmt.Errorf("reorder view card: %w", err)
	}

	// Propagate to parent board: reorder labeled cards in the matching parent lane.
	parentLanes, err := ReadLanes(parent.Dir)
	if err != nil {
		return fmt.Errorf("reorder view card: read parent lanes: %w", err)
	}
	parentLane, ok := LaneByName(parentLanes, lane.Name)
	if !ok {
		// view-only lane — no parent to update
		return nil
	}
	return ReorderCardAmongLabeled(parentLane, cardID, vb.FilterLabel, newIndex)
}

// RemoveCardFromView removes a card's FilterLabel from its parent card and
// deletes the stub from the view. This is the view-board equivalent of both
// "delete" and "archive" — the card itself is not deleted from the parent.
func RemoveCardFromView(vb *ViewBoard, parent *Board, cardID string) error {
	stub, _, err := FindViewCardStub(vb, cardID)
	if err != nil {
		return fmt.Errorf("remove card from view: %w", err)
	}

	// Remove filter label from the parent card.
	c, err := FindCard(parent, cardID, true)
	if err != nil {
		return fmt.Errorf("remove card from view: find in parent: %w", err)
	}
	next := make([]string, 0, len(c.Labels))
	for _, lid := range c.Labels {
		if lid != vb.FilterLabel {
			next = append(next, lid)
		}
	}
	c.Labels = next
	if err := WriteCard(c); err != nil {
		return fmt.Errorf("remove card from view: update parent card: %w", err)
	}

	// Remove stub from view.
	if err := os.Remove(stub.FilePath); err != nil {
		return fmt.Errorf("remove card from view: remove stub: %w", err)
	}
	return nil
}
