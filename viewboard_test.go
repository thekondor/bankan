package bankan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// newTestViewBoard creates a parent board with one label, one lane, and a view
// board filtered by that label. It does NOT add any cards.
func newTestViewBoard(t *testing.T) (*Board, *ViewBoard, Label) {
	t.Helper()
	parent := newTestBoard(t)
	lbl := Label{ID: "sprint", Name: "Sprint", Color: "#3b82f6"}
	require.NoError(t, AddLabel(parent, lbl))
	_, err := AddLane(parent, "Backlog")
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "sprint-view")
	vb, err := InitViewBoard(viewDir, "Sprint View", parent.Dir, lbl.ID)
	require.NoError(t, err)
	return parent, vb, lbl
}

// --- IsViewBoard ---

func TestIsViewBoard_True(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)
	assert.True(t, IsViewBoard(vb.Dir))
}

func TestIsViewBoard_False_RegularBoard(t *testing.T) {
	b := newTestBoard(t)
	assert.False(t, IsViewBoard(b.Dir))
}

func TestIsViewBoard_False_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsViewBoard(dir))
}

// --- InitViewBoard ---

func TestInitViewBoard_CreatesFiles(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "l1", Name: "Feature", Color: "#00ff00"}
	require.NoError(t, AddLabel(parent, lbl))

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "Feature View", parent.Dir, lbl.ID)
	require.NoError(t, err)

	assert.Equal(t, "Feature View", vb.Name)
	assert.Equal(t, parent.Dir, vb.Parent)
	assert.Equal(t, lbl.ID, vb.FilterLabel)
	assert.WithinDuration(t, time.Now(), vb.CreatedAt, 5*time.Second)
	assert.Nil(t, vb.ArchivedAt)

	// view.md exists.
	_, err = os.Stat(filepath.Join(viewDir, viewFileName))
	assert.NoError(t, err)

	// _archive dir exists.
	_, err = os.Stat(filepath.Join(viewDir, archiveDirName))
	assert.NoError(t, err)
}

func TestInitViewBoard_ClonesParentLanes(t *testing.T) {
	parent := newTestBoard(t)
	lbl := Label{ID: "l1", Name: "Bug", Color: "#ff0000"}
	require.NoError(t, AddLabel(parent, lbl))
	_, err := AddLane(parent, "Backlog")
	require.NoError(t, err)
	_, err = AddLane(parent, "In Progress")
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "view")
	_, err = InitViewBoard(viewDir, "Bug View", parent.Dir, lbl.ID)
	require.NoError(t, err)

	lanes, err := ReadLanes(viewDir)
	require.NoError(t, err)
	require.Len(t, lanes, 2)
	assert.Equal(t, "backlog", lanes[0].Name)
	assert.Equal(t, "in progress", lanes[1].Name)
}

func TestInitViewBoard_ErrorIfAlreadyExists(t *testing.T) {
	parent, vb, lbl := newTestViewBoard(t)
	_, err := InitViewBoard(vb.Dir, "Dup", parent.Dir, lbl.ID)
	assert.Error(t, err)
}

func TestInitViewBoard_ErrorIfNotABoard(t *testing.T) {
	notABoard := t.TempDir()
	viewDir := filepath.Join(t.TempDir(), "view")
	_, err := InitViewBoard(viewDir, "X", notABoard, "l1")
	assert.Error(t, err)
}

func TestInitViewBoard_ErrorIfLabelNotOnParent(t *testing.T) {
	parent := newTestBoard(t)
	viewDir := filepath.Join(t.TempDir(), "view")
	_, err := InitViewBoard(viewDir, "X", parent.Dir, "nonexistent")
	assert.Error(t, err)
}

// --- ReadViewBoard / WriteViewBoard ---

func TestReadViewBoard_RoundTrip(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)

	vb.Body = "My view description."
	require.NoError(t, WriteViewBoard(vb))

	vb2, err := ReadViewBoard(vb.Dir)
	require.NoError(t, err)

	assert.Equal(t, vb.Name, vb2.Name)
	assert.Equal(t, vb.Parent, vb2.Parent)
	assert.Equal(t, vb.FilterLabel, vb2.FilterLabel)
	assert.Equal(t, "My view description.\n", vb2.Body)
	assert.Nil(t, vb2.ArchivedAt)
}

// --- ArchiveViewBoard ---

func TestArchiveViewBoard_SetsArchivedAt(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)

	require.NoError(t, ArchiveViewBoard(vb))
	assert.NotNil(t, vb.ArchivedAt)
	assert.WithinDuration(t, time.Now(), *vb.ArchivedAt, 5*time.Second)

	// Persisted.
	vb2, err := ReadViewBoard(vb.Dir)
	require.NoError(t, err)
	require.NotNil(t, vb2.ArchivedAt)
	assert.WithinDuration(t, *vb.ArchivedAt, *vb2.ArchivedAt, 0)
}

func TestArchiveViewBoard_ErrorIfAlreadyArchived(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)
	require.NoError(t, ArchiveViewBoard(vb))
	err := ArchiveViewBoard(vb)
	assert.Error(t, err)
}

// --- FindViewBoard ---

func TestFindViewBoard_WalksUp(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)

	nested := filepath.Join(vb.Dir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	found, err := FindViewBoard(nested)
	require.NoError(t, err)
	assert.Equal(t, vb.Name, found.Name)
}

func TestFindViewBoard_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindViewBoard(dir)
	assert.Error(t, err)
}

// --- ParentBoard ---

func TestParentBoard_ReturnsParent(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	pb, err := ParentBoard(vb)
	require.NoError(t, err)
	assert.Equal(t, parent.Name, pb.Name)
	assert.Equal(t, parent.Dir, pb.Dir)
}

// --- IsBoard / IsViewBoard mutual exclusivity ---

func TestIsBoard_IsViewBoard_MutuallyExclusive(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	assert.True(t, IsBoard(parent.Dir))
	assert.False(t, IsViewBoard(parent.Dir))

	assert.True(t, IsViewBoard(vb.Dir))
	assert.False(t, IsBoard(vb.Dir))
}

func TestHideViewBoard_ShowViewBoard(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)
	assert.False(t, vb.Hidden)

	require.NoError(t, HideViewBoard(vb))
	assert.True(t, vb.Hidden)
	got, err := ReadViewBoard(vb.Dir)
	require.NoError(t, err)
	assert.True(t, got.Hidden)

	require.NoError(t, ShowViewBoard(vb))
	assert.False(t, vb.Hidden)
	got, err = ReadViewBoard(vb.Dir)
	require.NoError(t, err)
	assert.False(t, got.Hidden)
}
