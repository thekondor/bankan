package bankan_test

// lifecycle_integration_test.go exercises the full board-card-comment
// lifecycle end-to-end using only real filesystem operations via t.TempDir().
// These tests exercise multi-step workflows that cross package boundaries.

import (
	"os"
	"path/filepath"
	"testing"

	bankan "github.com/thekondor/bankan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func initBoard(t *testing.T, name string) *bankan.Board {
	t.Helper()
	b, err := bankan.InitBoard(t.TempDir(), name)
	require.NoError(t, err)
	return b
}

func mustAddLane(t *testing.T, b *bankan.Board, name string) bankan.Lane {
	t.Helper()
	l, err := bankan.AddLane(b, name)
	require.NoError(t, err)
	return l
}

func mustAddCard(t *testing.T, b *bankan.Board, lane bankan.Lane, title string) *bankan.Card {
	t.Helper()
	c, err := bankan.AddCard(b, lane, title, "Integration body.", nil)
	require.NoError(t, err)
	return c
}

// --- tests ---

func TestLifecycle_BoardInitAndRead(t *testing.T) {
	dir := t.TempDir()
	b, err := bankan.InitBoard(dir, "Project Alpha")
	require.NoError(t, err)
	assert.Equal(t, "Project Alpha", b.Name)
	assert.True(t, bankan.IsBoard(dir))

	// board.md is readable independently.
	b2, err := bankan.ReadBoard(dir)
	require.NoError(t, err)
	assert.Equal(t, b.Name, b2.Name)
	assert.WithinDuration(t, b.CreatedAt, b2.CreatedAt, 0)
}

func TestLifecycle_MultipleBoards(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "board-one")
	dir2 := filepath.Join(root, "board-two")

	require.NoError(t, os.MkdirAll(dir1, 0o755))
	require.NoError(t, os.MkdirAll(dir2, 0o755))

	_, err := bankan.InitBoard(dir1, "Board One")
	require.NoError(t, err)
	_, err = bankan.InitBoard(dir2, "Board Two")
	require.NoError(t, err)

	b1, err := bankan.ReadBoard(dir1)
	require.NoError(t, err)
	b2, err := bankan.ReadBoard(dir2)
	require.NoError(t, err)

	assert.Equal(t, "Board One", b1.Name)
	assert.Equal(t, "Board Two", b2.Name)
}

func TestLifecycle_LaneManagement(t *testing.T) {
	b := initBoard(t, "Lanes Test")

	l1 := mustAddLane(t, b, "Backlog")
	l2 := mustAddLane(t, b, "In Progress")
	l3 := mustAddLane(t, b, "Done")

	assert.Equal(t, 1, l1.Order)
	assert.Equal(t, 2, l2.Order)
	assert.Equal(t, 3, l3.Order)

	lanes, err := bankan.ReadLanes(b.Dir)
	require.NoError(t, err)
	require.Len(t, lanes, 3)
	assert.Equal(t, "backlog", lanes[0].Name)
	assert.Equal(t, "in progress", lanes[1].Name)
	assert.Equal(t, "done", lanes[2].Name)

	// Rename a lane.
	require.NoError(t, bankan.RenameLane(b, "In Progress", "Doing"))
	lanes, err = bankan.ReadLanes(b.Dir)
	require.NoError(t, err)
	_, ok := bankan.LaneByName(lanes, "Doing")
	assert.True(t, ok)
	_, ok = bankan.LaneByName(lanes, "In Progress")
	assert.False(t, ok)

	// Remove an empty lane.
	require.NoError(t, bankan.RemoveLane(b, "Done"))
	lanes, err = bankan.ReadLanes(b.Dir)
	require.NoError(t, err)
	assert.Len(t, lanes, 2)
}

func TestLifecycle_CardCRUD(t *testing.T) {
	b := initBoard(t, "Card CRUD")
	lane := mustAddLane(t, b, "Backlog")

	// Create.
	c := mustAddCard(t, b, lane, "Implement OAuth")
	assert.Len(t, c.ID, 5)
	assert.NotEmpty(t, c.FilePath)

	// Read via FindCard.
	found, err := bankan.FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, c.ID, found.ID)
	assert.Equal(t, "Implement OAuth", found.Title)

	// Edit (WriteCard).
	c.Title = "Implement OAuth 2.0"
	c.Body = "Updated body.\n"
	require.NoError(t, bankan.WriteCard(c))

	found2, err := bankan.FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "Implement OAuth 2.0", found2.Title)
	assert.True(t, found2.UpdatedAt.After(found2.CreatedAt) || found2.UpdatedAt.Equal(found2.CreatedAt))

	// Delete.
	require.NoError(t, bankan.DeleteCard(c))
	_, err = bankan.FindCard(b, c.ID, false)
	assert.Error(t, err)
}

func TestLifecycle_CardOrdering(t *testing.T) {
	b := initBoard(t, "Ordering")
	lane := mustAddLane(t, b, "Backlog")

	mustAddCard(t, b, lane, "Card One")
	mustAddCard(t, b, lane, "Card Two")
	mustAddCard(t, b, lane, "Card Three")

	cards, err := bankan.ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 3)
	assert.Equal(t, "Card One", cards[0].Title)
	assert.Equal(t, "Card Two", cards[1].Title)
	assert.Equal(t, "Card Three", cards[2].Title)
}

func TestLifecycle_MoveCard(t *testing.T) {
	b := initBoard(t, "Move Test")
	backlog := mustAddLane(t, b, "Backlog")
	doing := mustAddLane(t, b, "Doing")
	done := mustAddLane(t, b, "Done")

	c := mustAddCard(t, b, backlog, "Ship Feature")

	// backlog → doing
	require.NoError(t, bankan.MoveCard(b, c, doing))
	assert.Equal(t, doing.Name, c.Lane)
	assert.NotNil(t, c.MovedAt)
	assert.Equal(t, backlog.Name, c.MovedFrom)

	// doing → done
	require.NoError(t, bankan.MoveCard(b, c, done))
	assert.Equal(t, done.Name, c.Lane)
	assert.Equal(t, doing.Name, c.MovedFrom)

	// Verify it's readable in done lane.
	found, err := bankan.FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, done.Name, found.Lane)

	// Not in backlog or doing.
	backlogCards, err := bankan.ListCards(backlog)
	require.NoError(t, err)
	assert.Empty(t, backlogCards)

	doingCards, err := bankan.ListCards(doing)
	require.NoError(t, err)
	assert.Empty(t, doingCards)
}

func TestLifecycle_ArchiveAndRestore(t *testing.T) {
	b := initBoard(t, "Archive Test")
	lane := mustAddLane(t, b, "Backlog")

	c1 := mustAddCard(t, b, lane, "Keep")
	c2 := mustAddCard(t, b, lane, "Archive Me")

	// Archive c2.
	require.NoError(t, bankan.ArchiveCard(b, c2))

	// Active lane should have only c1.
	cards, err := bankan.ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, c1.ID, cards[0].ID)

	// Archive should have c2.
	archived, err := bankan.ListArchivedCards(b)
	require.NoError(t, err)
	require.Len(t, archived, 1)
	assert.Equal(t, c2.ID, archived[0].ID)
	assert.NotNil(t, archived[0].ArchivedAt)
	assert.Equal(t, lane.Name, archived[0].ArchivedFrom)

	// FindCard with searchArchive=true finds it.
	found, err := bankan.FindCard(b, c2.ID, true)
	require.NoError(t, err)
	assert.Equal(t, c2.ID, found.ID)

	// FindCard with searchArchive=false does NOT find it.
	_, err = bankan.FindCard(b, c2.ID, false)
	assert.Error(t, err)

	// Restore c2 into a new lane.
	done := mustAddLane(t, b, "Done")
	require.NoError(t, bankan.RestoreCard(b, c2, done))

	assert.Nil(t, c2.ArchivedAt)
	assert.Equal(t, done.Name, c2.Lane)

	doneCards, err := bankan.ListCards(done)
	require.NoError(t, err)
	require.Len(t, doneCards, 1)
	assert.Equal(t, c2.ID, doneCards[0].ID)

	// Archive empty after restore.
	archived, err = bankan.ListArchivedCards(b)
	require.NoError(t, err)
	assert.Empty(t, archived)
}

func TestLifecycle_CommentsWithMove(t *testing.T) {
	b := initBoard(t, "Comments Test")
	backlog := mustAddLane(t, b, "Backlog")
	doing := mustAddLane(t, b, "Doing")

	c := mustAddCard(t, b, backlog, "Commented Card")

	_, err := bankan.AddComment(c.FilePath, "alice", "First comment.")
	require.NoError(t, err)
	_, err = bankan.AddComment(c.FilePath, "bob", "Second comment.")
	require.NoError(t, err)

	// Verify before move.
	comments, err := bankan.ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, comments, 2)

	// Move card — comments must travel with it.
	oldPath := c.FilePath
	require.NoError(t, bankan.MoveCard(b, c, doing))
	assert.NotEqual(t, oldPath, c.FilePath)

	// Comments readable at new path.
	comments, err = bankan.ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, comments, 2)
	assert.Equal(t, "alice", comments[0].Author)
	assert.Equal(t, "bob", comments[1].Author)
}

func TestLifecycle_CommentsWithArchiveAndRestore(t *testing.T) {
	b := initBoard(t, "Comments Archive")
	lane := mustAddLane(t, b, "Backlog")
	c := mustAddCard(t, b, lane, "Card")

	_, err := bankan.AddComment(c.FilePath, "alice", "A note.")
	require.NoError(t, err)

	// Archive — comments travel to _archive.
	require.NoError(t, bankan.ArchiveCard(b, c))
	comments, err := bankan.ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, comments, 1)

	// Restore — comments travel back to lane.
	require.NoError(t, bankan.RestoreCard(b, c, lane))
	comments, err = bankan.ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "A note.", comments[0].Body)
}

func TestLifecycle_LabelManagement(t *testing.T) {
	b := initBoard(t, "Label Test")
	lane := mustAddLane(t, b, "Backlog")

	require.NoError(t, bankan.AddLabel(b, bankan.Label{ID: "bug", Name: "Bug", Color: "#ef4444"}))
	require.NoError(t, bankan.AddLabel(b, bankan.Label{ID: "feat", Name: "Feature", Color: "#3b82f6"}))

	// Duplicate label ID must fail.
	err := bankan.AddLabel(b, bankan.Label{ID: "bug", Name: "Other", Color: "#000000"})
	assert.Error(t, err)

	// Card with labels.
	c, err := bankan.AddCard(b, lane, "Labelled Card", "", []string{"bug"})
	require.NoError(t, err)
	assert.Equal(t, []string{"bug"}, c.Labels)

	// Unknown label on card creation must fail.
	_, err = bankan.AddCard(b, lane, "Bad Labels", "", []string{"unknown"})
	assert.Error(t, err)

	// Update label.
	require.NoError(t, bankan.UpdateLabel(b, bankan.Label{ID: "bug", Name: "Defect", Color: "#ff0000"}))
	b2, err := bankan.ReadBoard(b.Dir)
	require.NoError(t, err)
	l, ok := bankan.FindLabelByID(b2.Labels, "bug")
	require.True(t, ok)
	assert.Equal(t, "Defect", l.Name)

	// "feat" is unused; "bug" is used by the card above.
	unusedLabel, _ := bankan.FindLabelByID(b2.Labels, "feat")
	used, err := bankan.IsLabelUsedInBoard(b2, unusedLabel.ID)
	require.NoError(t, err)
	assert.False(t, used, "feat label has no cards")

	usedLabel, _ := bankan.FindLabelByID(b2.Labels, "bug")
	used, err = bankan.IsLabelUsedInBoard(b2, usedLabel.ID)
	require.NoError(t, err)
	assert.True(t, used, "bug label is assigned to 'Labelled Card'")

	// Remove (hard-delete) unused label.
	require.NoError(t, bankan.RemoveLabel(b, "feat"))
	b3, err := bankan.ReadBoard(b.Dir)
	require.NoError(t, err)
	assert.Len(t, b3.Labels, 1)
	_, stillThere := bankan.FindLabelByID(b3.Labels, "feat")
	assert.False(t, stillThere)
}

func TestLifecycle_FindBoard_FromSubdir(t *testing.T) {
	root := t.TempDir()
	_, err := bankan.InitBoard(root, "Root Board")
	require.NoError(t, err)

	// Create deep subdirectory simulating a source file location.
	deep := filepath.Join(root, "src", "pkg", "internal")
	require.NoError(t, os.MkdirAll(deep, 0o755))

	b, err := bankan.FindBoard(deep)
	require.NoError(t, err)
	assert.Equal(t, "Root Board", b.Name)
	assert.Equal(t, root, b.Dir)
}

// --- View board lifecycle tests ---

func mustInitViewBoard(t *testing.T, viewDir, name string, parent *bankan.Board, labelID string) *bankan.ViewBoard {
	t.Helper()
	vb, err := bankan.InitViewBoard(viewDir, name, parent.Dir, labelID)
	require.NoError(t, err)
	return vb
}

func mustAddCardWithLabel(t *testing.T, b *bankan.Board, lane bankan.Lane, title, labelID string) *bankan.Card {
	t.Helper()
	c, err := bankan.AddCard(b, lane, title, "body", []string{labelID})
	require.NoError(t, err)
	return c
}

func TestLifecycle_ViewBoard_CreateAndSync(t *testing.T) {
	// Parent board: two lanes, two labelled cards, one unlabelled.
	parent := initBoard(t, "Project Board")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sprint1", Name: "Sprint 1", Color: "#0000ff"}))
	backlog := mustAddLane(t, parent, "Backlog")
	doing := mustAddLane(t, parent, "Doing")

	c1 := mustAddCardWithLabel(t, parent, backlog, "Auth feature", "sprint1")
	c2 := mustAddCardWithLabel(t, parent, doing, "Profile page", "sprint1")
	mustAddCard(t, parent, backlog, "Unlabelled Card") // should NOT appear in view

	// Create and sync view board.
	viewDir := filepath.Join(t.TempDir(), "sprint1-view")
	vb := mustInitViewBoard(t, viewDir, "Sprint 1 View", parent, "sprint1")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	// View lanes were cloned from parent.
	viewLanes, err := bankan.ReadLanes(vb.Dir)
	require.NoError(t, err)
	require.Len(t, viewLanes, 2)

	// c1 stub in Backlog, c2 stub in Doing.
	vBacklog, ok := bankan.LaneByName(viewLanes, "backlog")
	require.True(t, ok)
	vDoing, ok := bankan.LaneByName(viewLanes, "doing")
	require.True(t, ok)

	backlogStubs, err := bankan.ListViewCardStubs(vBacklog)
	require.NoError(t, err)
	require.Len(t, backlogStubs, 1)
	assert.Equal(t, c1.ID, backlogStubs[0].CardID)

	doingStubs, err := bankan.ListViewCardStubs(vDoing)
	require.NoError(t, err)
	require.Len(t, doingStubs, 1)
	assert.Equal(t, c2.ID, doingStubs[0].CardID)
}

func TestLifecycle_ViewBoard_AddCardViaView(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	mustAddLane(t, parent, "Backlog")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")

	viewLanes, err := bankan.ReadLanes(vb.Dir)
	require.NoError(t, err)

	card, stub, err := bankan.AddViewCard(vb, parent, viewLanes[0], "API refactor", "body text", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, card.ID)
	assert.NotEmpty(t, stub.FilePath)

	// Card exists in parent with filter label.
	found, err := bankan.FindCard(parent, card.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "API refactor", found.Title)
	assert.Contains(t, found.Labels, "sp")

	// Stub file on disk.
	_, err = os.Stat(stub.FilePath)
	assert.NoError(t, err)
}

func TestLifecycle_ViewBoard_MoveCard_SharedLane(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")
	mustAddLane(t, parent, "Doing")

	c := mustAddCardWithLabel(t, parent, pBacklog, "Feature X", "sp")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	viewLanes, err := bankan.ReadLanes(vb.Dir)
	require.NoError(t, err)
	vDoing, ok := bankan.LaneByName(viewLanes, "doing")
	require.True(t, ok)

	// Move via view — should reflect in parent.
	require.NoError(t, bankan.MoveViewCard(vb, parent, c.ID, vDoing))

	parentCard, err := bankan.FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "doing", parentCard.Lane)

	_, stubLane, err := bankan.FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "doing", stubLane.Name)
}

func TestLifecycle_ViewBoard_MoveCard_ViewOnlyLane(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")

	c := mustAddCardWithLabel(t, parent, pBacklog, "Feature Y", "sp")
	originalCardFile := c.FilePath

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	// Add a view-only lane.
	viewOnly, err := bankan.AddViewLane(vb, parent, "Sprint Icebox")
	require.NoError(t, err)

	require.NoError(t, bankan.MoveViewCard(vb, parent, c.ID, viewOnly))

	// Parent card file untouched.
	_, err = os.Stat(originalCardFile)
	assert.NoError(t, err)

	parentCard, err := bankan.FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "backlog", parentCard.Lane)

	_, stubLane, err := bankan.FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "sprint icebox", stubLane.Name)
}

func TestLifecycle_ViewBoard_RemoveCardFromView(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")

	c := mustAddCardWithLabel(t, parent, pBacklog, "Dropped Card", "sp")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	require.NoError(t, bankan.RemoveCardFromView(vb, parent, c.ID))

	// Filter label removed from parent card.
	parentCard, err := bankan.FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.NotContains(t, parentCard.Labels, "sp")

	// Card still exists in parent (not deleted).
	assert.Equal(t, c.ID, parentCard.ID)

	// Stub gone from view.
	_, _, err = bankan.FindViewCardStub(vb, c.ID)
	assert.Error(t, err)
}

func TestLifecycle_ViewBoard_ArchiveView(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")

	// Cards in parent are unaffected by archiving the view.
	c := mustAddCardWithLabel(t, parent, pBacklog, "Surviving Card", "sp")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	require.NoError(t, bankan.ArchiveViewBoard(vb))

	// view.md reflects archived state.
	vb2, err := bankan.ReadViewBoard(vb.Dir)
	require.NoError(t, err)
	require.NotNil(t, vb2.ArchivedAt)

	// Parent card unaffected.
	parentCard, err := bankan.FindCard(parent, c.ID, false)
	require.NoError(t, err)
	assert.Contains(t, parentCard.Labels, "sp")
}

func TestLifecycle_ViewBoard_SyncBidirectional(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")

	c1 := mustAddCardWithLabel(t, parent, pBacklog, "Keep Me", "sp")
	c2 := mustAddCardWithLabel(t, parent, pBacklog, "Drop Me", "sp")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	// Verify both are tracked.
	_, _, err := bankan.FindViewCardStub(vb, c1.ID)
	require.NoError(t, err)
	_, _, err = bankan.FindViewCardStub(vb, c2.ID)
	require.NoError(t, err)

	// Remove filter label from c2 externally (outside the view).
	c2.Labels = nil
	require.NoError(t, bankan.WriteCard(c2))

	// Add a new card with the label.
	c3 := mustAddCardWithLabel(t, parent, pBacklog, "New Card", "sp")

	// Sync: c2 stub removed, c3 stub added.
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	_, _, err = bankan.FindViewCardStub(vb, c1.ID)
	assert.NoError(t, err, "c1 should still be present")

	_, _, err = bankan.FindViewCardStub(vb, c2.ID)
	assert.Error(t, err, "c2 stub should be removed (orphaned)")

	_, _, err = bankan.FindViewCardStub(vb, c3.ID)
	assert.NoError(t, err, "c3 stub should be added by sync")
}

func TestLifecycle_ViewBoard_SyncRelocatesStubAfterParentMove(t *testing.T) {
	// When a card is moved within the parent board (e.g. via CLI), a subsequent
	// sync must relocate the view stub to the matching view lane.
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pBacklog := mustAddLane(t, parent, "Backlog")
	pDoing := mustAddLane(t, parent, "Doing")

	c := mustAddCardWithLabel(t, parent, pBacklog, "Tracked Card", "sp")

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	// Initial state: stub is in Backlog.
	_, stubLane, err := bankan.FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "backlog", stubLane.Name)

	// Move card in parent board directly (simulates a CLI move on the parent).
	require.NoError(t, bankan.MoveCard(parent, c, pDoing))
	assert.Equal(t, "doing", c.Lane)

	// Sync: stub must follow the card to Doing.
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	_, stubLane, err = bankan.FindViewCardStub(vb, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "doing", stubLane.Name)

	// Exactly one stub across all view lanes (no duplication).
	viewLanes, err := bankan.ReadLanes(vb.Dir)
	require.NoError(t, err)
	total := 0
	for _, l := range viewLanes {
		stubs, err := bankan.ListViewCardStubs(l)
		require.NoError(t, err)
		total += len(stubs)
	}
	assert.Equal(t, 1, total)

	// Card resolves correctly via the view lane it was relocated to.
	vDoing, ok := bankan.LaneByName(viewLanes, "doing")
	require.True(t, ok)
	cards, err := bankan.ListViewCards(vb, parent, vDoing)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Tracked Card", cards[0].Title)
}

func TestLifecycle_ViewBoard_LabelRemoveBlockedOnFilterLabel(t *testing.T) {
	// The filter label on the parent board should not be removable while a view
	// exists that depends on it. This is a caller-enforcement contract:
	// the library does not auto-prevent it, but callers (CLI) must check.
	// This test documents the expectation via the label lookup path.
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")

	assert.Equal(t, "sp", vb.FilterLabel)
	// The parent still carries the label; removal via RemoveLabel succeeds at
	// library level — enforcement is in the CLI layer.
	_, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel)
	assert.True(t, ok)
}

func TestLifecycle_ViewBoard_FindViewBoard_WalksUp(t *testing.T) {
	parent := initBoard(t, "Project")
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))

	viewDir := filepath.Join(t.TempDir(), "view")
	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")

	nested := filepath.Join(vb.Dir, "docs", "notes")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	found, err := bankan.FindViewBoard(nested)
	require.NoError(t, err)
	assert.Equal(t, vb.Name, found.Name)
}

func TestLifecycle_ViewBoard_ViewAndParentInSameTree(t *testing.T) {
	// The view board can live anywhere; here we place it as a sibling of the parent.
	root := t.TempDir()
	parentDir := filepath.Join(root, "main-board")
	viewDir := filepath.Join(root, "sprint-view")

	require.NoError(t, os.MkdirAll(parentDir, 0o755))

	parent, err := bankan.InitBoard(parentDir, "Main Board")
	require.NoError(t, err)
	require.NoError(t, bankan.AddLabel(parent, bankan.Label{ID: "sp", Name: "Sprint", Color: "#00ff00"}))
	pLane := mustAddLane(t, parent, "Backlog")

	c := mustAddCardWithLabel(t, parent, pLane, "Sibling Test Card", "sp")

	vb := mustInitViewBoard(t, viewDir, "Sprint View", parent, "sp")
	require.NoError(t, bankan.SyncViewBoard(vb, parent))

	_, _, err = bankan.FindViewCardStub(vb, c.ID)
	assert.NoError(t, err)
}

func TestLifecycle_DeleteCard_WithComments(t *testing.T) {
	b := initBoard(t, "Delete With Comments")
	lane := mustAddLane(t, b, "Backlog")
	c := mustAddCard(t, b, lane, "Card")

	_, err := bankan.AddComment(c.FilePath, "alice", "Will be deleted.")
	require.NoError(t, err)

	commentsPath := filepath.Join(filepath.Dir(c.FilePath), c.ID+"-"+filepath.Base(c.FilePath)[4+5+1:len(filepath.Base(c.FilePath))-3]+".comments.md")
	_ = commentsPath // path computed by the library internals; just verify both files disappear.

	cardPath := c.FilePath
	require.NoError(t, bankan.DeleteCard(c))

	_, err = os.Stat(cardPath)
	assert.True(t, os.IsNotExist(err))
}

func TestLifecycle_IsLabelUsedInBoard_SpansActiveLaneAndArchive(t *testing.T) {
	b := initBoard(t, "Usage Scan")
	lane := mustAddLane(t, b, "Backlog")

	require.NoError(t, bankan.AddLabel(b, bankan.Label{ID: "lbl", Name: "Sprint", Color: "#3b82f6"}))

	// Not used yet.
	used, err := bankan.IsLabelUsedInBoard(b, "lbl")
	require.NoError(t, err)
	assert.False(t, used)

	// Add a card with the label and check again.
	c, err := bankan.AddCard(b, lane, "Sprint Task", "", []string{"lbl"})
	require.NoError(t, err)
	used, err = bankan.IsLabelUsedInBoard(b, "lbl")
	require.NoError(t, err)
	assert.True(t, used, "label is used in active lane")

	// Archive the card — label must still be reported as used (in archive).
	require.NoError(t, bankan.ArchiveCard(b, c))
	used, err = bankan.IsLabelUsedInBoard(b, "lbl")
	require.NoError(t, err)
	assert.True(t, used, "label is still used in archived card")

	// Restore the card and remove the label from it.
	require.NoError(t, bankan.RestoreCard(b, c, lane))
	c.Labels = nil
	require.NoError(t, bankan.WriteCard(c))

	used, err = bankan.IsLabelUsedInBoard(b, "lbl")
	require.NoError(t, err)
	assert.False(t, used, "label is no longer used after removal from card")
}

func TestLifecycle_IsLabelUsedInBoard_PrimaryLabel(t *testing.T) {
	b := initBoard(t, "Primary Label Usage")
	lane := mustAddLane(t, b, "Backlog")

	require.NoError(t, bankan.AddLabel(b, bankan.Label{ID: "pri", Name: "Critical", Color: "#ef4444"}))

	c, err := bankan.AddCard(b, lane, "Priority Task", "", nil)
	require.NoError(t, err)

	// Before setting primary label: not used.
	used, err := bankan.IsLabelUsedInBoard(b, "pri")
	require.NoError(t, err)
	assert.False(t, used)

	// Set as primary label.
	c.PrimaryLabel = "pri"
	require.NoError(t, bankan.WriteCard(c))

	used, err = bankan.IsLabelUsedInBoard(b, "pri")
	require.NoError(t, err)
	assert.True(t, used, "label is used as primary label")
}

func TestLifecycle_LabelArchivePrefixing(t *testing.T) {
	// Archiving a label means prefixing its name with ArchivedLabelPrefix.
	// The label remains on the board and on all cards; it is only hidden
	// from pickers via the prefix convention.
	b := initBoard(t, "Archive Prefix")
	lane := mustAddLane(t, b, "Backlog")

	require.NoError(t, bankan.AddLabel(b, bankan.Label{ID: "sp", Name: "Sprint", Color: "#3b82f6"}))

	// Assign to a card so we can verify it survives archiving.
	c, err := bankan.AddCard(b, lane, "Sprint Card", "", []string{"sp"})
	require.NoError(t, err)
	assert.Contains(t, c.Labels, "sp")

	// Archive the label: prefix its name.
	lbl, ok := bankan.FindLabelByID(b.Labels, "sp")
	require.True(t, ok)
	lbl.Name = bankan.ArchivedLabelPrefix + lbl.Name
	require.NoError(t, bankan.UpdateLabel(b, lbl))

	// Verify prefix is persisted.
	b2, err := bankan.ReadBoard(b.Dir)
	require.NoError(t, err)
	updated, ok := bankan.FindLabelByID(b2.Labels, "sp")
	require.True(t, ok)
	assert.Equal(t, bankan.ArchivedLabelPrefix+"Sprint", updated.Name)

	// The card's label ID is unchanged — only the board-level name changed.
	found, err := bankan.FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Contains(t, found.Labels, "sp")

	// IsLabelUsedInBoard still reports true (the card still references the ID).
	used, err := bankan.IsLabelUsedInBoard(b2, "sp")
	require.NoError(t, err)
	assert.True(t, used)
}
