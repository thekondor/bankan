package bankan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// newViewBoardWithCard creates a parent board with a labelled card in Backlog,
// and a view board filtered by that label.
// Returns parent board, view board, the label, and the card.
func newViewBoardWithCard(t *testing.T) (*Board, *ViewBoard, Label, *Card) {
	t.Helper()
	parent, vb, lbl := newTestViewBoard(t)

	// Add the labelled card to the parent.
	parentLanes, err := ReadLanes(parent.Dir)
	require.NoError(t, err)
	c, err := AddCard(parent, parentLanes[0], "Sprint Card", "body", []string{lbl.ID})
	require.NoError(t, err)

	return parent, vb, lbl, c
}

// --- ListViewCardStubs ---

func TestListViewCardStubs_Empty(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)

	lanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	require.NotEmpty(t, lanes)

	stubs, err := ListViewCardStubs(lanes[0])
	require.NoError(t, err)
	assert.Empty(t, stubs)
}

// --- SyncViewBoard ---

func TestSyncViewBoard_AddsNewStubs(t *testing.T) {
	parent, vb, _, c := newViewBoardWithCard(t)

	require.NoError(t, SyncViewBoard(vb, parent))

	stub, _, err := FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, stub.CardID)

	// Stub file exists on disk.
	_, err = os.Stat(stub.FilePath)
	assert.NoError(t, err)
}

func TestSyncViewBoard_Idempotent(t *testing.T) {
	parent, vb, _, _ := newViewBoardWithCard(t)

	require.NoError(t, SyncViewBoard(vb, parent))
	require.NoError(t, SyncViewBoard(vb, parent)) // second sync should not error or duplicate

	lanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	var total int
	for _, l := range lanes {
		stubs, err := ListViewCardStubs(l)
		require.NoError(t, err)
		total += len(stubs)
	}
	assert.Equal(t, 1, total)
}

func TestSyncViewBoard_RemovesOrphanedStubs(t *testing.T) {
	parent, vb, lbl, c := newViewBoardWithCard(t)
	require.NoError(t, SyncViewBoard(vb, parent))

	// Remove filter label from parent card — card is now "out of scope".
	c.Labels = nil
	require.NoError(t, WriteCard(c))

	require.NoError(t, SyncViewBoard(vb, parent))

	_, _, err := FindViewCardStub(vb, c.ID)
	assert.Error(t, err, "orphaned stub should have been removed")
	_ = lbl
}

func TestSyncViewBoard_PlacesStubInMatchingLane(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "bug", Name: "Bug", Color: "#ff0000"}
	require.NoError(t, AddLabel(parent, lbl))

	pBacklog, err := AddLane(parent, "Backlog")
	require.NoError(t, err)
	pDoing, err := AddLane(parent, "Doing")
	require.NoError(t, err)

	c1, err := AddCard(parent, pBacklog, "Card In Backlog", "", []string{lbl.ID})
	require.NoError(t, err)
	c2, err := AddCard(parent, pDoing, "Card In Doing", "", []string{lbl.ID})
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "Bug View", parent.Dir, lbl.ID)
	require.NoError(t, err)

	require.NoError(t, SyncViewBoard(vb, parent))

	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)

	backlogStubs, err := ListViewCardStubs(viewLanes[0]) // Backlog
	require.NoError(t, err)
	doingStubs, err := ListViewCardStubs(viewLanes[1]) // Doing
	require.NoError(t, err)

	require.Len(t, backlogStubs, 1)
	assert.Equal(t, c1.ID, backlogStubs[0].CardID)
	require.Len(t, doingStubs, 1)
	assert.Equal(t, c2.ID, doingStubs[0].CardID)
}

func TestSyncViewBoard_FallbackToFirstViewLane(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "tag", Name: "Tag", Color: "#aaaaaa"}
	require.NoError(t, AddLabel(parent, lbl))
	pLane, err := AddLane(parent, "Parent Only Lane")
	require.NoError(t, err)
	c, err := AddCard(parent, pLane, "Orphan Card", "", []string{lbl.ID})
	require.NoError(t, err)

	// View board has no lane named "Parent Only Lane".
	viewDir := filepath.Join(t.TempDir(), "view")
	// Manually create view with a different lane.
	vb, err := InitViewBoard(viewDir, "Tag View", parent.Dir, lbl.ID)
	require.NoError(t, err)

	// Remove the cloned lane (it was cloned from parent) and add a different one.
	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	for _, l := range viewLanes {
		require.NoError(t, os.Remove(l.Dir))
	}
	_, err = AddViewLane(vb, parent, "View Only Lane")
	require.NoError(t, err)

	require.NoError(t, SyncViewBoard(vb, parent))

	stub, _, err := FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, stub.CardID)
}

// --- ResolveViewCard ---

func TestResolveViewCard_ReturnsParentCard(t *testing.T) {
	parent, vb, _, c := newViewBoardWithCard(t)
	require.NoError(t, SyncViewBoard(vb, parent))

	stub, _, err := FindViewCardStub(vb, c.ID)
	require.NoError(t, err)

	resolved, err := ResolveViewCard(stub, parent)
	require.NoError(t, err)
	assert.Equal(t, c.ID, resolved.ID)
	assert.Equal(t, c.Title, resolved.Title)
}

// --- FindViewCardStub ---

func TestFindViewCardStub_NotFound(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)

	_, _, err := FindViewCardStub(vb, "xxxxx")
	assert.Error(t, err)
}

// --- AddViewCard ---

func TestAddViewCard_CreatesInParent(t *testing.T) {
	parent, vb, _, _ := newViewBoardWithCard(t)

	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)

	card, stub, err := AddViewCard(vb, parent, viewLanes[0], "New Sprint Card", "body", nil)
	require.NoError(t, err)

	// Card exists in parent.
	found, err := FindCard(parent, card.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "New Sprint Card", found.Title)

	// Filter label was auto-applied.
	assert.Contains(t, found.Labels, vb.FilterLabel)

	// Stub exists in view.
	_, err = os.Stat(stub.FilePath)
	assert.NoError(t, err)
}

func TestAddViewCard_AutoAppliesFilterLabel(t *testing.T) {
	parent, vb, lbl, _ := newViewBoardWithCard(t)

	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)

	card, _, err := AddViewCard(vb, parent, viewLanes[0], "Another Card", "", nil)
	require.NoError(t, err)

	found, err := FindCard(parent, card.ID, false)
	require.NoError(t, err)
	assert.Contains(t, found.Labels, lbl.ID)
}

func TestAddViewCard_FilterLabelNotDuplicated(t *testing.T) {
	parent, vb, lbl, _ := newViewBoardWithCard(t)

	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)

	// Pass the filter label explicitly in extraLabelIDs — should not duplicate.
	card, _, err := AddViewCard(vb, parent, viewLanes[0], "No Dup Card", "", []string{lbl.ID})
	require.NoError(t, err)

	found, err := FindCard(parent, card.ID, false)
	require.NoError(t, err)

	count := 0
	for _, l := range found.Labels {
		if l == lbl.ID {
			count++
		}
	}
	assert.Equal(t, 1, count, "filter label should appear exactly once")
}

// --- MoveViewCard ---

func TestMoveViewCard_ToSharedLane_ReflectsInParent(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "sp", Name: "Sprint", Color: "#0000ff"}
	require.NoError(t, AddLabel(parent, lbl))
	pBacklog, err := AddLane(parent, "Backlog")
	require.NoError(t, err)
	_, err = AddLane(parent, "Doing")
	require.NoError(t, err)

	c, err := AddCard(parent, pBacklog, "Move Me", "", []string{lbl.ID})
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "Sprint View", parent.Dir, lbl.ID)
	require.NoError(t, err)
	require.NoError(t, SyncViewBoard(vb, parent))

	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	vDoing, ok := LaneByName(viewLanes, "Doing")
	require.True(t, ok)

	require.NoError(t, MoveViewCard(vb, parent, c.ID, vDoing))

	// Parent card should be in Doing lane.
	found, err := FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "doing", found.Lane)

	// Stub should be in the view's Doing lane.
	stub, stubLane, err := FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "doing", stubLane.Name)
	assert.Equal(t, c.ID, stub.CardID)
}

func TestMoveViewCard_ToViewOnlyLane_DoesNotAffectParent(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "sp", Name: "Sprint", Color: "#0000ff"}
	require.NoError(t, AddLabel(parent, lbl))
	pBacklog, err := AddLane(parent, "Backlog")
	require.NoError(t, err)

	c, err := AddCard(parent, pBacklog, "View-Only Move", "", []string{lbl.ID})
	require.NoError(t, err)
	originalFile := c.FilePath

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "Sprint View", parent.Dir, lbl.ID)
	require.NoError(t, err)
	require.NoError(t, SyncViewBoard(vb, parent))

	// Add a view-only lane.
	viewOnlyLane, err := AddViewLane(vb, parent, "View Sprint Done")
	require.NoError(t, err)

	require.NoError(t, MoveViewCard(vb, parent, c.ID, viewOnlyLane))

	// Parent card file should be unchanged (still in Backlog).
	_, err = os.Stat(originalFile)
	assert.NoError(t, err, "parent card file should not have moved")

	found, err := FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "backlog", found.Lane, "parent lane should be unchanged")

	// Stub should now be in the view-only lane.
	_, stubLane, err := FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "view sprint done", stubLane.Name)
}

// --- RemoveCardFromView ---

func TestRemoveCardFromView_RemovesFilterLabelFromParent(t *testing.T) {
	parent, vb, lbl, c := newViewBoardWithCard(t)
	require.NoError(t, SyncViewBoard(vb, parent))

	require.NoError(t, RemoveCardFromView(vb, parent, c.ID))

	// Parent card no longer has filter label.
	found, err := FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.NotContains(t, found.Labels, lbl.ID)

	// Stub removed from view.
	_, _, err = FindViewCardStub(vb, c.ID)
	assert.Error(t, err)
}

func TestRemoveCardFromView_CardNotDeletedFromParent(t *testing.T) {
	parent, vb, _, c := newViewBoardWithCard(t)
	require.NoError(t, SyncViewBoard(vb, parent))

	require.NoError(t, RemoveCardFromView(vb, parent, c.ID))

	// Card itself still exists in parent.
	found, err := FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, c.ID, found.ID)
}

func TestRemoveCardFromView_PreservesOtherLabels(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "sprint", Name: "Sprint", Color: "#0000ff"}
	lbl2 := Label{ID: "bug", Name: "Bug", Color: "#ff0000"}
	require.NoError(t, AddLabel(parent, lbl))
	require.NoError(t, AddLabel(parent, lbl2))
	pLane, err := AddLane(parent, "Backlog")
	require.NoError(t, err)

	c, err := AddCard(parent, pLane, "Multi-labelled", "", []string{lbl.ID, lbl2.ID})
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "Sprint View", parent.Dir, lbl.ID)
	require.NoError(t, err)
	require.NoError(t, SyncViewBoard(vb, parent))

	require.NoError(t, RemoveCardFromView(vb, parent, c.ID))

	found, err := FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.NotContains(t, found.Labels, lbl.ID)
	assert.Contains(t, found.Labels, lbl2.ID)
}

// --- ListViewCards ---

func TestListViewCards_ReturnsResolvedCards(t *testing.T) {
	parent, vb, _, c := newViewBoardWithCard(t)
	require.NoError(t, SyncViewBoard(vb, parent))

	lanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)

	cards, err := ListViewCards(vb, parent, lanes[0])
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, c.ID, cards[0].ID)
	assert.Equal(t, c.Title, cards[0].Title)
	// Lane exposed is the view lane name.
	assert.Equal(t, lanes[0].Name, cards[0].Lane)
}

// --- ReorderViewCard ---

func TestReorderViewCard_ReordersStubsAndParent(t *testing.T) {
	parent, vb, lbl, c1 := newViewBoardWithCard(t)

	parentLanes, err := ReadLanes(parent.Dir)
	require.NoError(t, err)
	c2, err := AddCard(parent, parentLanes[0], "Sprint Card 2", "body", []string{lbl.ID})
	require.NoError(t, err)
	c3, err := AddCard(parent, parentLanes[0], "Sprint Card 3", "body", []string{lbl.ID})
	require.NoError(t, err)

	require.NoError(t, SyncViewBoard(vb, parent))

	// Move c1 (view index 0) to view index 2 (last).
	require.NoError(t, ReorderViewCard(vb, parent, c1.ID, 2))

	// Check view stub order.
	viewLanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	stubs, err := ListViewCardStubs(viewLanes[0])
	require.NoError(t, err)
	require.Len(t, stubs, 3)
	assert.Equal(t, c2.ID, stubs[0].CardID)
	assert.Equal(t, c3.ID, stubs[1].CardID)
	assert.Equal(t, c1.ID, stubs[2].CardID)

	// Check parent board labeled card order.
	parentLanes, err = ReadLanes(parent.Dir)
	require.NoError(t, err)
	cards, err := ListCards(parentLanes[0])
	require.NoError(t, err)
	var labeled []*Card
	for _, c := range cards {
		for _, lid := range c.Labels {
			if lid == lbl.ID {
				labeled = append(labeled, c)
				break
			}
		}
	}
	require.Len(t, labeled, 3)
	assert.Equal(t, c2.ID, labeled[0].ID)
	assert.Equal(t, c3.ID, labeled[1].ID)
	assert.Equal(t, c1.ID, labeled[2].ID)
}

func TestReorderViewCard_ViewOnlyLane_NoParentChange(t *testing.T) {
	parent, vb, lbl, _ := newViewBoardWithCard(t)

	// Add a view-only lane (not in parent) using AddViewLane.
	viewOnlyLane, err := AddViewLane(vb, parent, "View Only")
	require.NoError(t, err)

	// Add two labeled cards to parent and write stubs directly into the view-only lane.
	parentLanes, err := ReadLanes(parent.Dir)
	require.NoError(t, err)
	c1, err := AddCard(parent, parentLanes[0], "Card One", "body", []string{lbl.ID})
	require.NoError(t, err)
	c2, err := AddCard(parent, parentLanes[0], "Card Two", "body", []string{lbl.ID})
	require.NoError(t, err)

	_, err = writeViewCardStub(viewOnlyLane.Dir, c1.ID, "card-one", 1)
	require.NoError(t, err)
	_, err = writeViewCardStub(viewOnlyLane.Dir, c2.ID, "card-two", 2)
	require.NoError(t, err)

	// Reorder in view-only lane: should not error even though parent has no matching lane.
	require.NoError(t, ReorderViewCard(vb, parent, c1.ID, 1))

	// View stubs are reordered.
	stubs, err := ListViewCardStubs(viewOnlyLane)
	require.NoError(t, err)
	require.Len(t, stubs, 2)
	assert.Equal(t, c2.ID, stubs[0].CardID)
	assert.Equal(t, c1.ID, stubs[1].CardID)
}
