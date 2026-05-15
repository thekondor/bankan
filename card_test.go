package bankan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

func boardWithLane(t *testing.T) (*Board, Lane) {
	t.Helper()
	b := newTestBoard(t)
	lane, err := AddLane(b, "Backlog")
	require.NoError(t, err)
	return b, lane
}

func addTestCard(t *testing.T, b *Board, lane Lane, title string) *Card {
	t.Helper()
	c, err := AddCard(b, lane, title, "body text", nil)
	require.NoError(t, err)
	return c
}

// --- parseCardFilename ---

func TestParseCardFilename_Valid(t *testing.T) {
	order, id, slug, ok := parseCardFilename("001-ab12c-fix-login-bug.md")
	require.True(t, ok)
	assert.Equal(t, 1, order)
	assert.Equal(t, "ab12c", id)
	assert.Equal(t, "fix-login-bug", slug)
}

func TestParseCardFilename_Comments(t *testing.T) {
	_, _, _, ok := parseCardFilename("001-ab12c-fix-login-bug.comments.md")
	assert.False(t, ok, "comments file should not match card pattern")
}

func TestParseCardFilename_Invalid(t *testing.T) {
	_, _, _, ok := parseCardFilename("board.md")
	assert.False(t, ok)
}

func TestCommentFilename(t *testing.T) {
	assert.Equal(t, "001-ab12c-fix.comments.md", commentFilename("001-ab12c-fix.md"))
}

// --- AddCard ---

func TestAddCard_CreatesFile(t *testing.T) {
	b, lane := boardWithLane(t)
	c, err := AddCard(b, lane, "Fix login bug", "Some body.", nil)
	require.NoError(t, err)

	assert.Len(t, c.ID, idLength)
	assert.Equal(t, "Fix login bug", c.Title)
	assert.NotEmpty(t, c.FilePath)
	assert.WithinDuration(t, time.Now(), c.CreatedAt, 5*time.Second)

	_, err = os.Stat(c.FilePath)
	assert.NoError(t, err)
}

func TestAddCard_OrderPrefix(t *testing.T) {
	b, lane := boardWithLane(t)
	c1 := addTestCard(t, b, lane, "First")
	c2 := addTestCard(t, b, lane, "Second")

	o1, _, _, _ := parseCardFilename(filepath.Base(c1.FilePath))
	o2, _, _, _ := parseCardFilename(filepath.Base(c2.FilePath))
	assert.Equal(t, 1, o1)
	assert.Equal(t, 2, o2)
}

func TestAddCard_EmptyTitleError(t *testing.T) {
	b, lane := boardWithLane(t)
	_, err := AddCard(b, lane, "  ", "", nil)
	assert.Error(t, err)
}

func TestAddCard_InvalidLabelError(t *testing.T) {
	b, lane := boardWithLane(t)
	_, err := AddCard(b, lane, "Card", "", []string{"nonexistent"})
	assert.Error(t, err)
}

func TestAddCard_WithValidLabel(t *testing.T) {
	b, lane := boardWithLane(t)
	require.NoError(t, AddLabel(b, Label{ID: "l1", Name: "Bug", Color: "#ff0000"}))

	c, err := AddCard(b, lane, "Card", "", []string{"l1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"l1"}, c.Labels)
}

// --- ReadCard / WriteCard ---

func TestReadCard_Roundtrip(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "My Card")

	c2, err := ReadCard(c.FilePath)
	require.NoError(t, err)

	assert.Equal(t, c.ID, c2.ID)
	assert.Equal(t, c.Title, c2.Title)
	assert.Equal(t, "body text\n", c2.Body) // serialize adds trailing newline
}

func TestWriteCard_UpdatesUpdatedAt(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	original := c.UpdatedAt
	time.Sleep(10 * time.Millisecond)

	c.Title = "Updated Title"
	require.NoError(t, WriteCard(c))
	assert.True(t, c.UpdatedAt.After(original))
}

// --- ListCards ---

func TestListCards_SortedByOrder(t *testing.T) {
	b, lane := boardWithLane(t)
	addTestCard(t, b, lane, "Alpha")
	addTestCard(t, b, lane, "Beta")
	addTestCard(t, b, lane, "Gamma")

	cards, err := ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 3)
	assert.Equal(t, "Alpha", cards[0].Title)
	assert.Equal(t, "Beta", cards[1].Title)
	assert.Equal(t, "Gamma", cards[2].Title)
}

// --- FindCard ---

func TestFindCard_Found(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Find Me")

	found, err := FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, c.ID, found.ID)
}

func TestFindCard_NotFound(t *testing.T) {
	b, _ := boardWithLane(t)
	_, err := FindCard(b, "zzzzz", false)
	assert.Error(t, err)
}

// --- MoveCard ---

func TestMoveCard_ChangesLane(t *testing.T) {
	b, backlog := boardWithLane(t)
	doing, err := AddLane(b, "Doing")
	require.NoError(t, err)

	c := addTestCard(t, b, backlog, "Move Me")
	originalPath := c.FilePath

	require.NoError(t, MoveCard(b, c, doing))

	// File must be in the new lane directory.
	assert.Equal(t, doing.Dir, filepath.Dir(c.FilePath))
	// Old file must be gone.
	_, err = os.Stat(originalPath)
	assert.True(t, os.IsNotExist(err))

	// moved_at and moved_from set.
	assert.NotNil(t, c.MovedAt)
	assert.Equal(t, backlog.Name, c.MovedFrom)

	// Card is discoverable in new lane.
	found, err := FindCard(b, c.ID, false)
	require.NoError(t, err)
	assert.Equal(t, doing.Name, found.Lane)
}

func TestMoveCard_MovesCommentsFile(t *testing.T) {
	b, backlog := boardWithLane(t)
	doing, err := AddLane(b, "Doing")
	require.NoError(t, err)

	c := addTestCard(t, b, backlog, "Card With Comments")
	srcBase := filepath.Base(c.FilePath)
	commentsPath := filepath.Join(backlog.Dir, commentFilename(srcBase))

	// Create a comments file.
	require.NoError(t, os.WriteFile(commentsPath, []byte("# comments"), 0o644))

	require.NoError(t, MoveCard(b, c, doing))

	// Comments file must now be in the doing lane.
	newCommentsPath := filepath.Join(doing.Dir, commentFilename(filepath.Base(c.FilePath)))
	_, err = os.Stat(newCommentsPath)
	assert.NoError(t, err)

	// Old comments file must be gone.
	_, err = os.Stat(commentsPath)
	assert.True(t, os.IsNotExist(err))
}

// --- ArchiveCard / RestoreCard ---

func TestArchiveCard_MovesToArchive(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Archive Me")
	originalPath := c.FilePath

	require.NoError(t, ArchiveCard(b, c))

	assert.NotNil(t, c.ArchivedAt)
	assert.Equal(t, lane.Name, c.ArchivedFrom)

	// File must be in _archive.
	assert.Equal(t, archiveDir(b.Dir), filepath.Dir(c.FilePath))

	// Original file gone.
	_, err := os.Stat(originalPath)
	assert.True(t, os.IsNotExist(err))

	// Visible in ListArchivedCards.
	archived, err := ListArchivedCards(b)
	require.NoError(t, err)
	require.Len(t, archived, 1)
	assert.Equal(t, c.ID, archived[0].ID)
}

func TestArchiveCard_AlreadyArchived(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "X")
	require.NoError(t, ArchiveCard(b, c))
	err := ArchiveCard(b, c)
	assert.Error(t, err)
}

func TestRestoreCard_MovesBackToLane(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Restore Me")

	require.NoError(t, ArchiveCard(b, c))
	require.NoError(t, RestoreCard(b, c, lane))

	assert.Nil(t, c.ArchivedAt)
	assert.Equal(t, lane.Dir, filepath.Dir(c.FilePath))

	// Visible in lane, gone from archive.
	archived, err := ListArchivedCards(b)
	require.NoError(t, err)
	assert.Empty(t, archived)

	cards, err := ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, c.ID, cards[0].ID)
}

// --- DeleteCard ---

func TestDeleteCard_RemovesFile(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Delete Me")
	path := c.FilePath

	require.NoError(t, DeleteCard(c))

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteCard_RemovesCommentsFile(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	commentsPath := filepath.Join(lane.Dir, commentFilename(filepath.Base(c.FilePath)))
	require.NoError(t, os.WriteFile(commentsPath, []byte("# c"), 0o644))

	require.NoError(t, DeleteCard(c))

	_, err := os.Stat(commentsPath)
	assert.True(t, os.IsNotExist(err))
}

// --- ArchivedLabelNames ---

func TestArchiveCard_SnapshotsLabelNames(t *testing.T) {
	b, lane := boardWithLane(t)

	// Add two labels to the board.
	require.NoError(t, AddLabel(b, Label{ID: "aaa01", Name: "bug", Color: "#ff0000"}))
	require.NoError(t, WriteBoard(b))

	require.NoError(t, AddLabel(b, Label{ID: "bbb02", Name: "feature", Color: "#00ff00"}))
	require.NoError(t, WriteBoard(b))

	// Reload board so b.Labels is up to date.
	b, err := ReadBoard(b.Dir)
	require.NoError(t, err)

	// Create a card with both labels.
	c, err := AddCard(b, lane, "Labelled Card", "body", []string{"aaa01", "bbb02"})
	require.NoError(t, err)

	require.NoError(t, ArchiveCard(b, c))

	assert.Equal(t, []string{"bug", "feature"}, c.ArchivedLabelNames)

	// Round-trip: the snapshot must persist in the file.
	loaded, err := ReadCard(c.FilePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"bug", "feature"}, loaded.ArchivedLabelNames)
	assert.Equal(t, []string{"aaa01", "bbb02"}, loaded.Labels)
}

func TestArchiveCard_NoLabels_NoSnapshot(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Plain Card")

	require.NoError(t, ArchiveCard(b, c))

	assert.Nil(t, c.ArchivedLabelNames)
}

func TestRestoreCard_ClearsArchivedLabelNames(t *testing.T) {
	b, lane := boardWithLane(t)

	require.NoError(t, AddLabel(b, Label{ID: "ccc03", Name: "urgent", Color: "#ff0000"}))
	require.NoError(t, WriteBoard(b))
	b, err := ReadBoard(b.Dir)
	require.NoError(t, err)

	c, err := AddCard(b, lane, "Card", "body", []string{"ccc03"})
	require.NoError(t, err)

	require.NoError(t, ArchiveCard(b, c))
	require.Equal(t, []string{"urgent"}, c.ArchivedLabelNames)

	require.NoError(t, RestoreCard(b, c, lane))

	assert.Nil(t, c.ArchivedLabelNames)

	// Persisted file must not contain the snapshot.
	loaded, err := ReadCard(c.FilePath)
	require.NoError(t, err)
	assert.Nil(t, loaded.ArchivedLabelNames)
}

func TestCard_PrimaryLabel_PersistenceRoundtrip(t *testing.T) {
	b, lane := boardWithLane(t)
	require.NoError(t, AddLabel(b, Label{ID: "lbl01", Name: "Feature", Color: "#3b82f6"}))
	require.NoError(t, WriteBoard(b))
	b, err := ReadBoard(b.Dir)
	require.NoError(t, err)

	c, err := AddCard(b, lane, "My Card", "body", nil)
	require.NoError(t, err)
	assert.Empty(t, c.PrimaryLabel)

	c.PrimaryLabel = "lbl01"
	require.NoError(t, WriteCard(c))

	loaded, err := ReadCard(c.FilePath)
	require.NoError(t, err)
	assert.Equal(t, "lbl01", loaded.PrimaryLabel)
}

func TestCard_PrimaryLabel_ClearOnWrite(t *testing.T) {
	b, lane := boardWithLane(t)
	require.NoError(t, AddLabel(b, Label{ID: "lbl01", Name: "Feature", Color: "#3b82f6"}))
	require.NoError(t, WriteBoard(b))
	b, err := ReadBoard(b.Dir)
	require.NoError(t, err)

	c, err := AddCard(b, lane, "My Card", "body", nil)
	require.NoError(t, err)
	c.PrimaryLabel = "lbl01"
	require.NoError(t, WriteCard(c))

	c.PrimaryLabel = ""
	require.NoError(t, WriteCard(c))

	loaded, err := ReadCard(c.FilePath)
	require.NoError(t, err)
	assert.Empty(t, loaded.PrimaryLabel)
}

// --- DuplicateCard ---

func TestDuplicateCard_HappyPath(t *testing.T) {
	b, lane := boardWithLane(t)
	src := addTestCard(t, b, lane, "Original")

	dup, err := DuplicateCard(b, src)
	require.NoError(t, err)

	assert.Equal(t, "[dup] Original", dup.Title)
	assert.Equal(t, src.Body, dup.Body)
	assert.NotEqual(t, src.ID, dup.ID)
	assert.Equal(t, lane.Name, dup.Lane)
	assert.FileExists(t, dup.FilePath)

	// Comments file must NOT be created.
	commentsPath := filepath.Join(filepath.Dir(dup.FilePath), commentFilename(filepath.Base(dup.FilePath)))
	_, err = os.Stat(commentsPath)
	assert.True(t, os.IsNotExist(err), "comments file must not exist for a new duplicate")
}

func TestDuplicateCard_WithPrimaryLabel(t *testing.T) {
	b, lane := boardWithLane(t)
	require.NoError(t, AddLabel(b, Label{ID: "lbl01", Name: "Feature", Color: "#3b82f6"}))
	require.NoError(t, WriteBoard(b))
	b, err := ReadBoard(b.Dir)
	require.NoError(t, err)

	src, err := AddCard(b, lane, "Tagged", "body", []string{"lbl01"})
	require.NoError(t, err)
	src.PrimaryLabel = "lbl01"
	require.NoError(t, WriteCard(src))

	dup, err := DuplicateCard(b, src)
	require.NoError(t, err)

	assert.Equal(t, []string{"lbl01"}, dup.Labels)
	assert.Equal(t, "lbl01", dup.PrimaryLabel)

	// Verify persistence.
	loaded, err := ReadCard(dup.FilePath)
	require.NoError(t, err)
	assert.Equal(t, "lbl01", loaded.PrimaryLabel)
}

func TestDuplicateCard_SourceLaneNotFound(t *testing.T) {
	b, lane := boardWithLane(t)
	src := addTestCard(t, b, lane, "Orphaned")
	src.Lane = "nonexistent-lane"

	_, err := DuplicateCard(b, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source lane")
}

// --- ReorderCard ---

func TestReorderCard_MovesForward(t *testing.T) {
	b, lane := boardWithLane(t)
	c1 := addTestCard(t, b, lane, "First")
	c2 := addTestCard(t, b, lane, "Second")
	c3 := addTestCard(t, b, lane, "Third")

	// Move c1 (index 0) to index 2 (last).
	require.NoError(t, ReorderCard(lane, c1.ID, 2))

	cards, err := ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 3)
	assert.Equal(t, c2.ID, cards[0].ID)
	assert.Equal(t, c3.ID, cards[1].ID)
	assert.Equal(t, c1.ID, cards[2].ID)
}

func TestReorderCard_MovesBackward(t *testing.T) {
	b, lane := boardWithLane(t)
	c1 := addTestCard(t, b, lane, "First")
	c2 := addTestCard(t, b, lane, "Second")
	c3 := addTestCard(t, b, lane, "Third")

	// Move c3 (index 2) to index 0 (first).
	require.NoError(t, ReorderCard(lane, c3.ID, 0))

	cards, err := ListCards(lane)
	require.NoError(t, err)
	require.Len(t, cards, 3)
	assert.Equal(t, c3.ID, cards[0].ID)
	assert.Equal(t, c1.ID, cards[1].ID)
	assert.Equal(t, c2.ID, cards[2].ID)
}

func TestReorderCard_SameIndex_NoOp(t *testing.T) {
	b, lane := boardWithLane(t)
	c1 := addTestCard(t, b, lane, "First")
	c2 := addTestCard(t, b, lane, "Second")

	require.NoError(t, ReorderCard(lane, c1.ID, 0))

	cards, err := ListCards(lane)
	require.NoError(t, err)
	assert.Equal(t, c1.ID, cards[0].ID)
	assert.Equal(t, c2.ID, cards[1].ID)
}

func TestReorderCard_MovesCommentsFile(t *testing.T) {
	b, lane := boardWithLane(t)
	c1 := addTestCard(t, b, lane, "First")
	c2 := addTestCard(t, b, lane, "Second")

	// Create a comments file for c1.
	commentsPath := filepath.Join(lane.Dir, commentFilename(filepath.Base(c1.FilePath)))
	require.NoError(t, os.WriteFile(commentsPath, []byte("# comments"), 0o644))

	require.NoError(t, ReorderCard(lane, c1.ID, 1))

	// c1 is now at index 1; its comments file must have been renamed.
	cards, err := ListCards(lane)
	require.NoError(t, err)
	assert.Equal(t, c2.ID, cards[0].ID)
	assert.Equal(t, c1.ID, cards[1].ID)

	newCommentsPath := filepath.Join(lane.Dir, commentFilename(filepath.Base(cards[1].FilePath)))
	_, err = os.Stat(newCommentsPath)
	assert.NoError(t, err, "comments file should exist at new path")
	_, err = os.Stat(commentsPath)
	assert.True(t, os.IsNotExist(err), "old comments file should be gone")
}

// --- ReorderCardAmongLabeled ---

func TestReorderCardAmongLabeled_ReordersLabeledSubset(t *testing.T) {
	b, lane := boardWithLane(t)
	require.NoError(t, AddLabel(b, Label{ID: "lbl1", Name: "Sprint", Color: "#ff0000"}))

	// 3 labeled, 1 unlabeled.
	c1, _ := AddCard(b, lane, "Labeled A", "", []string{"lbl1"})
	addTestCard(t, b, lane, "Unlabeled")
	c3, _ := AddCard(b, lane, "Labeled B", "", []string{"lbl1"})
	c4, _ := AddCard(b, lane, "Labeled C", "", []string{"lbl1"})

	// Move c1 (index 0 in labeled) to index 2 (last in labeled).
	require.NoError(t, ReorderCardAmongLabeled(lane, c1.ID, "lbl1", 2))

	cards, err := ListCards(lane)
	require.NoError(t, err)

	// Collect labeled cards in their new order.
	var labeled []*Card
	for _, c := range cards {
		for _, lid := range c.Labels {
			if lid == "lbl1" {
				labeled = append(labeled, c)
				break
			}
		}
	}
	require.Len(t, labeled, 3)
	assert.Equal(t, c3.ID, labeled[0].ID)
	assert.Equal(t, c4.ID, labeled[1].ID)
	assert.Equal(t, c1.ID, labeled[2].ID)
}
