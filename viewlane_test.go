package bankan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- AddViewLane ---

func TestAddViewLane_CreatesDirectory(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	lane, err := AddViewLane(vb, parent, "Review")
	require.NoError(t, err)

	_, err = os.Stat(lane.Dir)
	assert.NoError(t, err)
	assert.Equal(t, "review", lane.Name)
}

func TestAddViewLane_OrderAppended(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	// The view was initialised with parent's "Backlog" lane (order 1).
	l2, err := AddViewLane(vb, parent, "Review")
	require.NoError(t, err)
	l3, err := AddViewLane(vb, parent, "Done")
	require.NoError(t, err)

	assert.Equal(t, 2, l2.Order)
	assert.Equal(t, 3, l3.Order)
}

func TestAddViewLane_ErrorOnDuplicateInView(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	_, err := AddViewLane(vb, parent, "Review")
	require.NoError(t, err)

	_, err = AddViewLane(vb, parent, "review") // case-insensitive
	assert.Error(t, err)
}

func TestAddViewLane_ErrorOnNameExistsInParent(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	// "Backlog" was cloned from the parent into the view during InitViewBoard.
	// Adding it as a view-only lane should fail because it exists in the parent.
	_, err := AddViewLane(vb, parent, "Backlog")
	assert.Error(t, err, "lane name clashes with parent lane")
}

func TestAddViewLane_AllowsNameNotInParent(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	_, err := AddViewLane(vb, parent, "Icebox")
	assert.NoError(t, err, "new name not in parent should be allowed")
}

func TestAddViewLane_ErrorOnEmptyName(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)
	_, err := AddViewLane(vb, parent, "")
	assert.Error(t, err)
}

// --- RemoveViewLane ---

func TestRemoveViewLane_EmptyLane(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	// Add a view-only lane to remove.
	_, err := AddViewLane(vb, parent, "Icebox")
	require.NoError(t, err)

	err = RemoveViewLane(vb, "Icebox")
	require.NoError(t, err)

	lanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	_, ok := LaneByName(lanes, "Icebox")
	assert.False(t, ok)
}

func TestRemoveViewLane_ErrorIfNotFound(t *testing.T) {
	_, vb, _ := newTestViewBoard(t)
	err := RemoveViewLane(vb, "Nonexistent")
	assert.Error(t, err)
}

func TestRemoveViewLane_NonEmptyLane_RemovesStubs(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	// Add a view-only lane and plant a fake stub file.
	lane, err := AddViewLane(vb, parent, "Icebox")
	require.NoError(t, err)

	stubPath := filepath.Join(lane.Dir, "001-ab12c-card.md")
	f, err := os.Create(stubPath)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Removing a non-empty view lane succeeds and deletes the stub.
	err = RemoveViewLane(vb, "Icebox")
	require.NoError(t, err)

	lanes, err := ReadLanes(vb.Dir)
	require.NoError(t, err)
	_, ok := LaneByName(lanes, "Icebox")
	assert.False(t, ok, "lane should be gone from view")
	_, statErr := os.Stat(stubPath)
	assert.True(t, os.IsNotExist(statErr), "stub file should have been removed")
}

func TestRemoveViewLane_DoesNotAffectParent(t *testing.T) {
	parent, vb, _ := newTestViewBoard(t)

	// Remove the cloned "Backlog" lane from the view.
	err := RemoveViewLane(vb, "backlog")
	require.NoError(t, err)

	// Parent still has its lane.
	parentLanes, err := ReadLanes(parent.Dir)
	require.NoError(t, err)
	_, ok := LaneByName(parentLanes, "backlog")
	assert.True(t, ok)
}

// --- Combined: AddViewLane uniqueness across full combined set ---

func TestAddViewLane_CombinedUniqueness(t *testing.T) {
	// Parent: Backlog, In Progress
	// View (after init): Backlog, In Progress (cloned)
	// Adding "In Progress" to view should fail (exists in parent).
	parent := newTestBoard(t)
	lbl := Label{ID: "t", Name: "Tag", Color: "#aabbcc"}
	require.NoError(t, AddLabel(parent, lbl))
	_, err := AddLane(parent, "Backlog")
	require.NoError(t, err)
	_, err = AddLane(parent, "In Progress")
	require.NoError(t, err)

	viewDir := filepath.Join(t.TempDir(), "view")
	vb, err := InitViewBoard(viewDir, "V", parent.Dir, lbl.ID)
	require.NoError(t, err)

	_, err = AddViewLane(vb, parent, "In Progress")
	assert.Error(t, err, "In Progress exists in parent, must be rejected")

	// A brand-new name is fine.
	_, err = AddViewLane(vb, parent, "Icebox")
	assert.NoError(t, err)
}
