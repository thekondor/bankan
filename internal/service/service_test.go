package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/internal/service"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func setupBoard(t *testing.T) (string, *service.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	_, err := bankan.InitBoard(dir, "Test Board")
	require.NoError(t, err)
	reg, id, err := service.NewSingleRegistry(dir)
	require.NoError(t, err)
	return dir, reg, id
}

func setupBoardWithLane(t *testing.T) (string, *service.Registry, string, bankan.Lane) {
	t.Helper()
	dir, reg, id := setupBoard(t)
	lane, err := reg.AddLane(id, "Backlog")
	require.NoError(t, err)
	return dir, reg, id, lane
}

func setupViewBoard(t *testing.T) (parentDir string, viewDir string, reg *service.Registry, parentID string, viewID string, label bankan.Label) {
	t.Helper()
	parentDir = t.TempDir()
	_, err := bankan.InitBoard(parentDir, "Parent Board")
	require.NoError(t, err)

	// Add a lane to parent.
	parentReg, parentID, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	_, err = parentReg.AddLane(parentID, "Backlog")
	require.NoError(t, err)

	// Add a label to parent.
	lbl, err := parentReg.AddLabel(parentID, "feature", "#3b82f6")
	require.NoError(t, err)
	label = lbl

	// Create view board.
	viewDir = t.TempDir()
	_, err = bankan.InitViewBoard(viewDir, "Sprint View", parentDir, lbl.ID)
	require.NoError(t, err)

	// Build a registry with both boards.
	reg, err = service.NewRegistry([]string{parentDir, viewDir}, "")
	require.NoError(t, err)
	parentID = filepath.Base(parentDir)
	viewID = filepath.Base(viewDir)
	return
}

// ─── Registry ────────────────────────────────────────────────────────────────

func TestRegistry_NewSingleRegistry(t *testing.T) {
	dir, reg, id := setupBoard(t)
	assert.Equal(t, filepath.Base(dir), id)
	assert.False(t, reg.IsViewBoard(id))
	assert.Contains(t, reg.BoardIDs(), id)
}

func TestRegistry_NewRegistry_Scan(t *testing.T) {
	parent := t.TempDir()
	board1 := filepath.Join(parent, "board-one")
	board2 := filepath.Join(parent, "board-two")
	require.NoError(t, os.MkdirAll(board1, 0o755))
	require.NoError(t, os.MkdirAll(board2, 0o755))
	_, err := bankan.InitBoard(board1, "One")
	require.NoError(t, err)
	_, err = bankan.InitBoard(board2, "Two")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{parent}, "")
	require.NoError(t, err)
	ids := reg.BoardIDs()
	assert.Contains(t, ids, "board-one")
	assert.Contains(t, ids, "board-two")
	assert.Equal(t, parent, reg.RootDir())
}

func TestRegistry_DuplicateID_Error(t *testing.T) {
	dir := t.TempDir()
	_, err := bankan.InitBoard(dir, "Board")
	require.NoError(t, err)
	reg, _, err := service.NewSingleRegistry(dir)
	require.NoError(t, err)
	_, err = reg.Register(dir)
	var conflictErr *service.ErrConflict
	assert.True(t, errors.As(err, &conflictErr))
}

func TestRegistry_NotABoard_Error(t *testing.T) {
	dir := t.TempDir()
	_, _, err := service.NewSingleRegistry(dir)
	assert.Error(t, err)
}

func TestRegistry_BoardNotFound_Error(t *testing.T) {
	_, reg, _ := setupBoard(t)
	_, err := reg.GetBoard("nonexistent")
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

// ─── Board operations ─────────────────────────────────────────────────────────

func TestRegistry_GetBoard(t *testing.T) {
	_, reg, id := setupBoard(t)
	b, err := reg.GetBoard(id)
	require.NoError(t, err)
	assert.Equal(t, "Test Board", b.Name)
}

func TestRegistry_InitBoard(t *testing.T) {
	rootDir := t.TempDir()
	reg, err := service.NewRegistry([]string{}, rootDir)
	require.NoError(t, err)
	b, err := reg.InitBoard("my-new-board")
	require.NoError(t, err)
	assert.Equal(t, "my-new-board", b.Name)
	assert.Contains(t, reg.BoardIDs(), "my-new-board")
}

func TestRegistry_InitBoard_NoRootDir_Forbidden(t *testing.T) {
	reg, err := service.NewRegistry([]string{}, "")
	require.NoError(t, err)
	_, err = reg.InitBoard("something")
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

// ─── Lane operations ─────────────────────────────────────────────────────────

func TestRegistry_AddLane(t *testing.T) {
	_, reg, id := setupBoard(t)
	lane, err := reg.AddLane(id, "Backlog")
	require.NoError(t, err)
	// Lane names are lowercased by the library (deslugify preserves no case).
	assert.Equal(t, "backlog", lane.Name)
	assert.Equal(t, 1, lane.Order)
}

func TestRegistry_ListLanes(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	_, err := reg.AddLane(id, "In Progress")
	require.NoError(t, err)

	lanes, err := reg.ListLanes(id)
	require.NoError(t, err)
	require.Len(t, lanes, 2)
	assert.Equal(t, "backlog", lanes[0].Name)
	assert.Equal(t, "in progress", lanes[1].Name)
}

func TestRegistry_RenameLane(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	err := reg.RenameLane(id, "Backlog", "Todo")
	require.NoError(t, err)
	lanes, err := reg.ListLanes(id)
	require.NoError(t, err)
	assert.Equal(t, "todo", lanes[0].Name)
}

func TestRegistry_RemoveLane_Empty(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	err := reg.RemoveLane(id, "Backlog")
	require.NoError(t, err)
	lanes, err := reg.ListLanes(id)
	require.NoError(t, err)
	assert.Empty(t, lanes)
}

func TestRegistry_RemoveLane_NonEmpty_Error(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	_, err := reg.AddCard(id, "Backlog", "My Card", "", nil)
	require.NoError(t, err)
	err = reg.RemoveLane(id, "Backlog")
	assert.Error(t, err)
}

// ─── Card operations ─────────────────────────────────────────────────────────

func TestRegistry_AddCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Fix login bug", "Some body", nil)
	require.NoError(t, err)
	assert.Equal(t, "Fix login bug", c.Title)
	assert.NotEmpty(t, c.ID)
}

func TestRegistry_AddCard_InvalidLane(t *testing.T) {
	_, reg, id := setupBoard(t)
	_, err := reg.AddCard(id, "Nonexistent", "Card", "", nil)
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

func TestRegistry_GetCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "My card", "", nil)
	require.NoError(t, err)
	got, err := reg.GetCard(id, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, "My card", got.Title)
}

func TestRegistry_GetCard_NotFound(t *testing.T) {
	_, reg, id := setupBoard(t)
	_, err := reg.GetCard(id, "zzzzz")
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

func TestRegistry_UpdateCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Old title", "old body", nil)
	require.NoError(t, err)

	newTitle := "New title"
	newBody := "new body"
	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{
		Title: &newTitle,
		Body:  &newBody,
	})
	require.NoError(t, err)
	assert.Equal(t, "New title", updated.Title)
	assert.Equal(t, "new body", updated.Body)
}

func TestRegistry_UpdateCard_AddRemoveLabel(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "bug", "#ef4444")
	require.NoError(t, err)

	c, err := reg.AddCard(id, "Backlog", "Card with label", "", []string{lbl.ID})
	require.NoError(t, err)
	assert.Contains(t, c.Labels, lbl.ID)

	// Remove the label.
	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{RemoveLabels: []string{lbl.ID}})
	require.NoError(t, err)
	assert.NotContains(t, updated.Labels, lbl.ID)
}

func TestRegistry_MoveCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	_, err := reg.AddLane(id, "In Progress")
	require.NoError(t, err)

	c, err := reg.AddCard(id, "Backlog", "Move me", "", nil)
	require.NoError(t, err)

	err = reg.MoveCard(id, c.ID, "In Progress")
	require.NoError(t, err)

	moved, err := reg.GetCard(id, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "in progress", moved.Lane)
}

func TestRegistry_ArchiveAndRestoreCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Card to archive", "", nil)
	require.NoError(t, err)

	err = reg.ArchiveCard(id, c.ID)
	require.NoError(t, err)

	archived, err := reg.ListArchivedCards(id)
	require.NoError(t, err)
	require.Len(t, archived, 1)
	assert.Equal(t, c.ID, archived[0].ID)

	err = reg.RestoreCard(id, c.ID, "Backlog")
	require.NoError(t, err)

	restored, err := reg.GetCard(id, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "backlog", restored.Lane)
	assert.Nil(t, restored.ArchivedAt)
}

func TestRegistry_DeleteCard(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Delete me", "", nil)
	require.NoError(t, err)

	err = reg.DeleteCard(id, c.ID)
	require.NoError(t, err)

	_, err = reg.GetCard(id, c.ID)
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

func TestRegistry_ListAllCards(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	_, err := reg.AddLane(id, "Done")
	require.NoError(t, err)
	_, err = reg.AddCard(id, "Backlog", "Card A", "", nil)
	require.NoError(t, err)
	_, err = reg.AddCard(id, "Done", "Card B", "", nil)
	require.NoError(t, err)

	all, err := reg.ListAllCards(id)
	require.NoError(t, err)
	// Lane names come back lowercase from the library.
	assert.Len(t, all["backlog"], 1)
	assert.Len(t, all["done"], 1)
}

// ─── Comment operations ───────────────────────────────────────────────────────

func TestRegistry_AddAndListComments(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	_, err = reg.AddComment(id, c.ID, "alice", "First comment")
	require.NoError(t, err)
	_, err = reg.AddComment(id, c.ID, "bob", "Second comment")
	require.NoError(t, err)

	comments, err := reg.ListComments(id, c.ID)
	require.NoError(t, err)
	require.Len(t, comments, 2)
	assert.Equal(t, "alice", comments[0].Author)
	assert.Equal(t, "bob", comments[1].Author)
}

func TestRegistry_ListComments_Empty(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	comments, err := reg.ListComments(id, c.ID)
	require.NoError(t, err)
	assert.Empty(t, comments)
}

func TestRegistry_UpdateComment(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	cm, err := reg.AddComment(id, c.ID, "alice", "Original body")
	require.NoError(t, err)
	_, err = reg.AddComment(id, c.ID, "bob", "Another comment")
	require.NoError(t, err)

	updated, err := reg.UpdateComment(id, c.ID, cm.ID, "Edited body")
	require.NoError(t, err)
	assert.Equal(t, cm.ID, updated.ID)
	assert.Equal(t, "Edited body", updated.Body)
	assert.Equal(t, "alice", updated.Author)

	all, err := reg.ListComments(id, c.ID)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "Edited body", all[0].Body)
	assert.Equal(t, "Another comment", all[1].Body)
}

func TestRegistry_UpdateComment_NotFound(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)
	_, _ = reg.AddComment(id, c.ID, "alice", "Some comment")

	_, err = reg.UpdateComment(id, c.ID, "zzzzz", "New body")
	assert.Error(t, err)
}

// ─── Label operations ─────────────────────────────────────────────────────────

func TestRegistry_AddListLabel(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)
	assert.Equal(t, "Bug", lbl.Name)
	assert.Equal(t, "#ef4444", lbl.Color)

	labels, err := reg.ListLabels(id)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, lbl.ID, labels[0].ID)
}

func TestRegistry_UpdateLabel(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)

	newName := "Defect"
	newColor := "#f97316"
	updated, err := reg.UpdateLabel(id, lbl.ID, service.LabelUpdate{
		Name:  &newName,
		Color: &newColor,
	})
	require.NoError(t, err)
	assert.Equal(t, "Defect", updated.Name)
	assert.Equal(t, "#f97316", updated.Color)
}

func TestRegistry_RemoveLabel_Force(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Tag", "#3b82f6")
	require.NoError(t, err)

	err = reg.RemoveLabel(id, lbl.ID, true)
	require.NoError(t, err)

	labels, err := reg.ListLabels(id)
	require.NoError(t, err)
	assert.Empty(t, labels)
}

func TestRegistry_RemoveLabel_DefaultArchives(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Tag", "#3b82f6")
	require.NoError(t, err)

	err = reg.RemoveLabel(id, lbl.ID, false)
	require.NoError(t, err)

	labels, err := reg.ListLabels(id)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, bankan.ArchivedLabelPrefix+"Tag", labels[0].Name)
}

func TestRegistry_AddLabel_ViewBoard_Forbidden(t *testing.T) {
	_, _, reg, _, viewID, _ := setupViewBoard(t)
	_, err := reg.AddLabel(viewID, "Tag", "#3b82f6")
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

// ─── View board operations ────────────────────────────────────────────────────

func TestRegistry_ViewBoard_GetViewBoard(t *testing.T) {
	_, _, reg, _, viewID, _ := setupViewBoard(t)
	vb, parent, err := reg.GetViewBoard(viewID)
	require.NoError(t, err)
	assert.Equal(t, "Sprint View", vb.Name)
	assert.Equal(t, "Parent Board", parent.Name)
}

func TestRegistry_ViewBoard_AddCard(t *testing.T) {
	_, _, reg, parentID, viewID, lbl := setupViewBoard(t)
	c, err := reg.AddCard(viewID, "Backlog", "View card", "body", []string{lbl.ID})
	require.NoError(t, err)
	assert.Equal(t, "View card", c.Title)
	// The card should be visible in the parent too.
	got, err := reg.GetCard(parentID, c.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Labels, lbl.ID)
}

func TestRegistry_ViewBoard_ArchiveCard_RemovesFromView(t *testing.T) {
	_, _, reg, parentID, viewID, lbl := setupViewBoard(t)
	c, err := reg.AddCard(viewID, "Backlog", "View card", "", nil)
	require.NoError(t, err)

	err = reg.ArchiveCard(viewID, c.ID)
	require.NoError(t, err)

	// Parent card should no longer have the filter label.
	parentCard, err := reg.GetCard(parentID, c.ID)
	require.NoError(t, err)
	assert.NotContains(t, parentCard.Labels, lbl.ID)
}

func TestRegistry_ViewBoard_RestoreCard_Forbidden(t *testing.T) {
	_, _, reg, _, viewID, _ := setupViewBoard(t)
	err := reg.RestoreCard(viewID, "ab12c", "Backlog")
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

func TestRegistry_ViewBoard_RemoveFilterLabel_Forbidden(t *testing.T) {
	_, _, reg, _, viewID, lbl := setupViewBoard(t)
	err := reg.RemoveLabel(viewID, lbl.ID, true)
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

func TestRegistry_ViewBoard_SyncViewBoard(t *testing.T) {
	_, _, reg, parentID, viewID, lbl := setupViewBoard(t)
	// Add a card with filter label directly to parent (not via view).
	c, err := reg.AddCard(parentID, "Backlog", "Direct parent card", "", []string{lbl.ID})
	require.NoError(t, err)

	err = reg.SyncViewBoard(viewID)
	require.NoError(t, err)

	// Card should now be visible in view.
	got, err := reg.GetCard(viewID, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
}

func TestRegistry_ViewBoard_ArchiveViewBoard(t *testing.T) {
	_, _, reg, _, viewID, _ := setupViewBoard(t)
	err := reg.ArchiveViewBoard(viewID, false)
	require.NoError(t, err)

	vb, _, err := reg.GetViewBoard(viewID)
	require.NoError(t, err)
	assert.NotNil(t, vb.ArchivedAt)
}

// ─── InitBoard slugification ─────────────────────────────────────────────────

func TestRegistry_InitBoard_SlugifiesName(t *testing.T) {
	rootDir := t.TempDir()
	reg, err := service.NewRegistry([]string{}, rootDir)
	require.NoError(t, err)

	b, err := reg.InitBoard("My Project")
	require.NoError(t, err)

	// Directory name must be the slugified form.
	assert.Equal(t, "my-project", filepath.Base(b.Dir))
	// Display name is preserved as-is.
	assert.Equal(t, "My Project", b.Name)
	// Registry ID equals the slugified dirname.
	assert.Contains(t, reg.BoardIDs(), "my-project")
}

// ─── InitViewBoard ────────────────────────────────────────────────────────────

func TestRegistry_InitViewBoard_HappyPath(t *testing.T) {
	rootDir := t.TempDir()

	// Create a parent board in rootDir.
	parentDir := filepath.Join(rootDir, "parent")
	_, err := bankan.InitBoard(parentDir, "Parent Board")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{parentDir}, rootDir)
	require.NoError(t, err)
	parentID := filepath.Base(parentDir)

	// Add a label to the parent board so we have a valid filter label.
	lbl, err := reg.AddLabel(parentID, "sprint", "#3b82f6")
	require.NoError(t, err)

	// Create the view board.
	vb, err := reg.InitViewBoard("Sprint One", parentID, lbl.ID)
	require.NoError(t, err)

	// Directory name must be slugified.
	assert.Equal(t, "sprint-one", filepath.Base(vb.Dir))
	// Display name is preserved.
	assert.Equal(t, "Sprint One", vb.Name)
	// Filter label stored correctly.
	assert.Equal(t, lbl.ID, vb.FilterLabel)
	// Registered as a view board.
	viewID := filepath.Base(vb.Dir)
	assert.True(t, reg.IsViewBoard(viewID))
}

func TestRegistry_InitViewBoard_NoRootDir_Forbidden(t *testing.T) {
	parentDir := t.TempDir()
	_, err := bankan.InitBoard(parentDir, "Parent")
	require.NoError(t, err)

	// Registry with no rootDir.
	reg, err := service.NewRegistry([]string{parentDir}, "")
	require.NoError(t, err)
	parentID := filepath.Base(parentDir)

	_, err = reg.InitViewBoard("Sprint", parentID, "some-label")
	require.Error(t, err)
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

func TestRegistry_InitViewBoard_ParentIsViewBoard_Forbidden(t *testing.T) {
	rootDir := t.TempDir()

	// Create a regular parent board.
	parentDir := filepath.Join(rootDir, "parent")
	_, err := bankan.InitBoard(parentDir, "Parent")
	require.NoError(t, err)

	// Create a view board of the parent.
	viewDir := filepath.Join(rootDir, "view-one")
	parentReg, parentID, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	lbl, err := parentReg.AddLabel(parentID, "feat", "#ff0000")
	require.NoError(t, err)
	_, err = bankan.InitViewBoard(viewDir, "View One", parentDir, lbl.ID)
	require.NoError(t, err)

	// Multi-board registry that knows about all three directories.
	reg, err := service.NewRegistry([]string{parentDir, viewDir}, rootDir)
	require.NoError(t, err)
	viewID := filepath.Base(viewDir)

	// Attempting to create a view board of a view board must fail.
	_, err = reg.InitViewBoard("Nested View", viewID, lbl.ID)
	require.Error(t, err)
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden))
}

func TestRegistry_UpdateCard_SetPrimaryLabel(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "Feature", "#3b82f6")
	require.NoError(t, err)
	lbl2, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)

	// Card starts with only Bug as a regular label; Feature is not yet assigned.
	c, err := reg.AddCard(id, "Backlog", "Card", "", []string{lbl2.ID})
	require.NoError(t, err)
	assert.Empty(t, c.PrimaryLabel)

	// Setting Feature as primary must also add it to regular labels.
	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &lbl.ID})
	require.NoError(t, err)
	assert.Equal(t, lbl.ID, updated.PrimaryLabel)
	assert.Contains(t, updated.Labels, lbl.ID, "primary label must be added to regular labels")
	assert.Contains(t, updated.Labels, lbl2.ID, "other regular labels must remain")
}

func TestRegistry_UpdateCard_SetPrimaryLabel_AlreadyRegular(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "Feature", "#3b82f6")
	require.NoError(t, err)

	// Card already has the label as a regular label.
	c, err := reg.AddCard(id, "Backlog", "Card", "", []string{lbl.ID})
	require.NoError(t, err)

	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &lbl.ID})
	require.NoError(t, err)
	assert.Equal(t, lbl.ID, updated.PrimaryLabel)
	// Must not be duplicated in Labels.
	count := 0
	for _, l := range updated.Labels {
		if l == lbl.ID {
			count++
		}
	}
	assert.Equal(t, 1, count, "label must appear exactly once in regular labels")
}

func TestRegistry_UpdateCard_ClearPrimaryLabel(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "Feature", "#3b82f6")
	require.NoError(t, err)

	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	// Set primary label first.
	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &lbl.ID})
	require.NoError(t, err)
	assert.Equal(t, lbl.ID, updated.PrimaryLabel)

	// Clear primary label with empty string.
	empty := ""
	cleared, err := reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &empty})
	require.NoError(t, err)
	assert.Empty(t, cleared.PrimaryLabel)
}

func TestRegistry_UpdateCard_PrimaryLabel_InvalidID(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)

	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	nonExistent := "zzzzz"
	_, err = reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &nonExistent})
	require.Error(t, err)
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

func TestRegistry_UpdateCard_PrimaryLabel_NilNoChange(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "Feature", "#3b82f6")
	require.NoError(t, err)

	c, err := reg.AddCard(id, "Backlog", "Card", "", nil)
	require.NoError(t, err)

	// Set primary label.
	updated, err := reg.UpdateCard(id, c.ID, service.CardUpdate{PrimaryLabel: &lbl.ID})
	require.NoError(t, err)
	assert.Equal(t, lbl.ID, updated.PrimaryLabel)

	// Update title only (PrimaryLabel=nil) must not touch primary label.
	newTitle := "Updated title"
	noChange, err := reg.UpdateCard(id, c.ID, service.CardUpdate{Title: &newTitle})
	require.NoError(t, err)
	assert.Equal(t, "Updated title", noChange.Title)
	assert.Equal(t, lbl.ID, noChange.PrimaryLabel, "primary label must remain unchanged")
}

// ─── DuplicateCard ────────────────────────────────────────────────────────────

func TestRegistry_DuplicateCard_HappyPath(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)
	src, err := reg.AddCard(id, "Backlog", "Original", "body text", nil)
	require.NoError(t, err)

	dup, err := reg.DuplicateCard(id, src.ID)
	require.NoError(t, err)

	assert.Equal(t, "[dup] Original", dup.Title)
	assert.Equal(t, "body text\n", dup.Body) // body roundtrips through disk; serialize adds trailing newline
	assert.NotEqual(t, src.ID, dup.ID)
	assert.Equal(t, src.Lane, dup.Lane)
}

func TestRegistry_DuplicateCard_ViewBoardForbidden(t *testing.T) {
	_, _, reg, _, viewID, _ := setupViewBoard(t)

	// Add a card to view first.
	_, err := reg.AddCard(viewID, "Backlog", "Card", "", nil)
	require.NoError(t, err)
	cards, err := reg.ListCards(viewID, "backlog")
	require.NoError(t, err)
	require.NotEmpty(t, cards)

	_, err = reg.DuplicateCard(viewID, cards[0].ID)
	var forbidden *service.ErrForbidden
	assert.True(t, errors.As(err, &forbidden), "expected ErrForbidden for view board")
}

func TestRegistry_DuplicateCard_NotFound(t *testing.T) {
	_, reg, id, _ := setupBoardWithLane(t)

	_, err := reg.DuplicateCard(id, "nope0")
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound), "expected ErrNotFound for unknown card ID")
}

// ─── Board ordering ───────────────────────────────────────────────────────────

func TestRegistry_BoardIDs_SortsByOrder(t *testing.T) {
	root := t.TempDir()
	boardA := filepath.Join(root, "board-a")
	boardB := filepath.Join(root, "board-b")
	boardC := filepath.Join(root, "board-c")
	require.NoError(t, os.MkdirAll(boardA, 0o755))
	require.NoError(t, os.MkdirAll(boardB, 0o755))
	require.NoError(t, os.MkdirAll(boardC, 0o755))

	bA, err := bankan.InitBoard(boardA, "A")
	require.NoError(t, err)
	bB, err := bankan.InitBoard(boardB, "B")
	require.NoError(t, err)
	bC, err := bankan.InitBoard(boardC, "C")
	require.NoError(t, err)

	// Set explicit order: C=1, A=2, B=3.
	bC.Order = 1
	require.NoError(t, bankan.WriteBoard(bC))
	bA.Order = 2
	require.NoError(t, bankan.WriteBoard(bA))
	bB.Order = 3
	require.NoError(t, bankan.WriteBoard(bB))

	reg, err := service.NewRegistry([]string{root}, "")
	require.NoError(t, err)

	ids := reg.BoardIDs()
	require.Equal(t, []string{"board-c", "board-a", "board-b"}, ids)
}

func TestRegistry_BoardIDs_UnorderedSortsLast(t *testing.T) {
	root := t.TempDir()
	boardOrdered := filepath.Join(root, "board-ordered")
	boardUnordered := filepath.Join(root, "board-unordered")
	require.NoError(t, os.MkdirAll(boardOrdered, 0o755))
	require.NoError(t, os.MkdirAll(boardUnordered, 0o755))

	bOrd, err := bankan.InitBoard(boardOrdered, "Ordered")
	require.NoError(t, err)
	bOrd.Order = 1
	require.NoError(t, bankan.WriteBoard(bOrd))

	_, err = bankan.InitBoard(boardUnordered, "Unordered") // order stays 0
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{root}, "")
	require.NoError(t, err)

	ids := reg.BoardIDs()
	require.Equal(t, []string{"board-ordered", "board-unordered"}, ids)
}

func TestRegistry_ReorderBoards_PersistsOrder(t *testing.T) {
	root := t.TempDir()
	boardA := filepath.Join(root, "board-a")
	boardB := filepath.Join(root, "board-b")
	require.NoError(t, os.MkdirAll(boardA, 0o755))
	require.NoError(t, os.MkdirAll(boardB, 0o755))
	_, err := bankan.InitBoard(boardA, "A")
	require.NoError(t, err)
	_, err = bankan.InitBoard(boardB, "B")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{root}, "")
	require.NoError(t, err)

	// Reorder: B first, A second.
	require.NoError(t, reg.ReorderBoards([]string{"board-b", "board-a"}))
	ids := reg.BoardIDs()
	assert.Equal(t, []string{"board-b", "board-a"}, ids)

	// Verify persisted to disk.
	bA, err := bankan.ReadBoard(boardA)
	require.NoError(t, err)
	assert.Equal(t, 2, bA.Order)

	bB, err := bankan.ReadBoard(boardB)
	require.NoError(t, err)
	assert.Equal(t, 1, bB.Order)

	// Reload registry — order must survive restart.
	reg2, err := service.NewRegistry([]string{root}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"board-b", "board-a"}, reg2.BoardIDs())
}

func TestRegistry_ReorderBoards_NotFound(t *testing.T) {
	_, reg, _ := setupBoard(t)
	err := reg.ReorderBoards([]string{"does-not-exist"})
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}

func TestRegistry_InitBoard_AssignsOrder(t *testing.T) {
	root := t.TempDir()
	reg, err := service.NewRegistry([]string{root}, root)
	require.NoError(t, err)

	b1, err := reg.InitBoard("First")
	require.NoError(t, err)
	assert.Equal(t, 1, b1.Order)

	b2, err := reg.InitBoard("Second")
	require.NoError(t, err)
	assert.Equal(t, 2, b2.Order)
}

func TestRegistry_HideBoard_ShowBoard(t *testing.T) {
	dir, reg, id := setupBoard(t)

	assert.False(t, reg.IsHiddenBoard(id))

	require.NoError(t, reg.HideBoard(id))
	assert.True(t, reg.IsHiddenBoard(id))
	b, err := bankan.ReadBoard(dir)
	require.NoError(t, err)
	assert.True(t, b.Hidden)

	require.NoError(t, reg.ShowBoard(id))
	assert.False(t, reg.IsHiddenBoard(id))
	b, err = bankan.ReadBoard(dir)
	require.NoError(t, err)
	assert.False(t, b.Hidden)
}

func TestRegistry_HideViewBoard_ShowViewBoard(t *testing.T) {
	_, viewDir, reg, _, viewID, _ := setupViewBoard(t)

	assert.False(t, reg.IsHiddenBoard(viewID))

	require.NoError(t, reg.HideBoard(viewID))
	assert.True(t, reg.IsHiddenBoard(viewID))
	vb, err := bankan.ReadViewBoard(viewDir)
	require.NoError(t, err)
	assert.True(t, vb.Hidden)

	require.NoError(t, reg.ShowBoard(viewID))
	assert.False(t, reg.IsHiddenBoard(viewID))
	vb, err = bankan.ReadViewBoard(viewDir)
	require.NoError(t, err)
	assert.False(t, vb.Hidden)
}

// ─── SearchCard ───────────────────────────────────────────────────────────────

func TestSearchCard_FoundInOneBoard(t *testing.T) {
	container := t.TempDir()
	boardDir := filepath.Join(container, "my-board")
	require.NoError(t, os.MkdirAll(boardDir, 0o755))
	_, err := bankan.InitBoard(boardDir, "My Board")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{container}, "")
	require.NoError(t, err)
	boardID := filepath.Base(boardDir)

	_, err = reg.AddLane(boardID, "Backlog")
	require.NoError(t, err)
	card, err := reg.AddCard(boardID, "Backlog", "Fix login", "", nil)
	require.NoError(t, err)

	results, err := reg.SearchCard(card.ID, false)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "My Board", results[0].BoardName)
	assert.Equal(t, "backlog", results[0].LaneName)
	assert.Equal(t, "Fix login", results[0].CardTitle)
}

func TestSearchCard_NotFound(t *testing.T) {
	container := t.TempDir()
	boardDir := filepath.Join(container, "empty-board")
	require.NoError(t, os.MkdirAll(boardDir, 0o755))
	_, err := bankan.InitBoard(boardDir, "Empty Board")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{container}, "")
	require.NoError(t, err)
	boardID := filepath.Base(boardDir)
	_, err = reg.AddLane(boardID, "Backlog")
	require.NoError(t, err)

	results, err := reg.SearchCard("zzzzz", false)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchCard_FoundInViewBoard(t *testing.T) {
	// setupViewBoard creates a parent board ("Parent Board") with "Backlog" lane + filter label,
	// and a view board ("Sprint View") filtering by that label. Both are registered in reg.
	_, _, reg, parentID, viewID, label := setupViewBoard(t)
	_ = parentID

	// Add a view-only lane "Sprint 1" (view lane names must not collide with parent lanes).
	_, err := reg.AddLane(viewID, "Sprint 1")
	require.NoError(t, err)

	// Add card via the view board in "Sprint 1":
	// - card is created in the parent's first lane ("backlog") with the filter label applied
	// - a stub is placed in the view's "sprint 1" lane
	card, err := reg.AddCard(viewID, "Sprint 1", "Implement OAuth", "", nil)
	require.NoError(t, err)
	_ = label

	// The card should be found in both the parent board and the view board.
	results, err := reg.SearchCard(card.ID, false)
	require.NoError(t, err)
	require.Len(t, results, 2, "card must appear in both parent board and view board")

	boardNames := make([]string, len(results))
	for i, r := range results {
		boardNames[i] = r.BoardName
		assert.Equal(t, "Implement OAuth", r.CardTitle)
	}
	assert.Contains(t, boardNames, "Parent Board")
	assert.Contains(t, boardNames, "Sprint View")
}

func TestSearchCard_ArchivedCardNotFound(t *testing.T) {
	container := t.TempDir()
	boardDir := filepath.Join(container, "board-with-archive")
	require.NoError(t, os.MkdirAll(boardDir, 0o755))
	_, err := bankan.InitBoard(boardDir, "Archive Board")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{container}, "")
	require.NoError(t, err)
	boardID := filepath.Base(boardDir)

	_, err = reg.AddLane(boardID, "Done")
	require.NoError(t, err)
	card, err := reg.AddCard(boardID, "Done", "Archived task", "", nil)
	require.NoError(t, err)

	require.NoError(t, reg.ArchiveCard(boardID, card.ID))

	results, err := reg.SearchCard(card.ID, false)
	require.NoError(t, err)
	assert.Empty(t, results, "archived cards must not be returned by search")
}

func TestSearchCard_SkipsArchivedViewBoard(t *testing.T) {
	_, _, reg, parentID, viewID, label := setupViewBoard(t)
	_ = parentID

	_, err := reg.AddLane(viewID, "Sprint 1")
	require.NoError(t, err)

	card, err := reg.AddCard(viewID, "Sprint 1", "Some task", "", nil)
	require.NoError(t, err)
	_ = label

	// Archive the view board.
	require.NoError(t, reg.ArchiveViewBoard(viewID, false))

	// Default search (includeArchived=false) must skip the archived view board.
	results, err := reg.SearchCard(card.ID, false)
	require.NoError(t, err)
	for _, r := range results {
		assert.NotEqual(t, viewID, r.BoardID, "archived view board must be excluded from default search")
	}
}

func TestSearchCard_IncludesArchivedViewBoardWithFlag(t *testing.T) {
	_, _, reg, parentID, viewID, label := setupViewBoard(t)
	_ = parentID

	_, err := reg.AddLane(viewID, "Sprint 1")
	require.NoError(t, err)

	card, err := reg.AddCard(viewID, "Sprint 1", "Some task", "", nil)
	require.NoError(t, err)
	_ = label

	// Archive the view board.
	require.NoError(t, reg.ArchiveViewBoard(viewID, false))

	// Search with includeArchived=true must find the card in the view board.
	results, err := reg.SearchCard(card.ID, true)
	require.NoError(t, err)

	boardIDs := make([]string, len(results))
	for i, r := range results {
		boardIDs[i] = r.BoardID
	}
	assert.Contains(t, boardIDs, viewID, "archived view board must be included when flag is set")
}

func TestRegistry_IsLabelUsed_NotUsed(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)

	used, err := reg.IsLabelUsed(id, lbl.ID)
	require.NoError(t, err)
	assert.False(t, used)
}

func TestRegistry_IsLabelUsed_UsedInCard(t *testing.T) {
	_, reg, id, lane := setupBoardWithLane(t)
	lbl, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)
	_, err = reg.AddCard(id, lane.Name, "Task", "", []string{lbl.ID})
	require.NoError(t, err)

	used, err := reg.IsLabelUsed(id, lbl.ID)
	require.NoError(t, err)
	assert.True(t, used)
}

func TestRegistry_ArchiveLabel(t *testing.T) {
	_, reg, id := setupBoard(t)
	lbl, err := reg.AddLabel(id, "Bug", "#ef4444")
	require.NoError(t, err)

	require.NoError(t, reg.ArchiveLabel(id, lbl.ID))

	labels, err := reg.ListLabels(id)
	require.NoError(t, err)
	var found bankan.Label
	for _, l := range labels {
		if l.ID == lbl.ID {
			found = l
		}
	}
	assert.Equal(t, bankan.ArchivedLabelPrefix+"Bug", found.Name)
}

func TestRegistry_ArchiveLabel_NotFound(t *testing.T) {
	_, reg, id := setupBoard(t)
	err := reg.ArchiveLabel(id, "nonexistent")
	require.Error(t, err)
	var notFound *service.ErrNotFound
	assert.True(t, errors.As(err, &notFound))
}
