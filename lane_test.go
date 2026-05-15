package bankan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestBoard(t *testing.T) *Board {
	t.Helper()
	dir := t.TempDir()
	b, err := InitBoard(dir, "Test")
	require.NoError(t, err)
	return b
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Backlog", "backlog"},
		{"In Progress", "in-progress"},
		{"  Done!! ", "done"},
		{"To-Do", "to-do"},
		{"a--b", "a-b"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, slugify(tc.in), "slugify(%q)", tc.in)
	}
}

func TestParseLaneDir(t *testing.T) {
	order, name, ok := parseLaneDir("01-backlog")
	require.True(t, ok)
	assert.Equal(t, 1, order)
	assert.Equal(t, "backlog", name)

	order, name, ok = parseLaneDir("03-in-progress")
	require.True(t, ok)
	assert.Equal(t, 3, order)
	assert.Equal(t, "in progress", name)

	_, _, ok = parseLaneDir("_archive")
	assert.False(t, ok)

	_, _, ok = parseLaneDir("board.md")
	assert.False(t, ok)
}

func TestAddLane_CreatesDirectory(t *testing.T) {
	b := newTestBoard(t)

	lane, err := AddLane(b, "Backlog")
	require.NoError(t, err)
	assert.Equal(t, 1, lane.Order)

	_, err = os.Stat(lane.Dir)
	assert.NoError(t, err)
}

func TestAddLane_OrderIncrement(t *testing.T) {
	b := newTestBoard(t)

	l1, err := AddLane(b, "Backlog")
	require.NoError(t, err)
	l2, err := AddLane(b, "In Progress")
	require.NoError(t, err)
	l3, err := AddLane(b, "Done")
	require.NoError(t, err)

	assert.Equal(t, 1, l1.Order)
	assert.Equal(t, 2, l2.Order)
	assert.Equal(t, 3, l3.Order)
}

func TestAddLane_DuplicateNameError(t *testing.T) {
	b := newTestBoard(t)
	_, err := AddLane(b, "Backlog")
	require.NoError(t, err)
	_, err = AddLane(b, "backlog") // case-insensitive
	assert.Error(t, err)
}

func TestReadLanes_SortedByOrder(t *testing.T) {
	b := newTestBoard(t)
	require.NoError(t, os.Mkdir(filepath.Join(b.Dir, "03-done"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(b.Dir, "01-backlog"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(b.Dir, "02-in-progress"), 0o755))

	lanes, err := ReadLanes(b.Dir)
	require.NoError(t, err)
	require.Len(t, lanes, 3)
	assert.Equal(t, 1, lanes[0].Order)
	assert.Equal(t, 2, lanes[1].Order)
	assert.Equal(t, 3, lanes[2].Order)
}

func TestReadLanes_IgnoresNonLaneDirs(t *testing.T) {
	b := newTestBoard(t)
	_, err := AddLane(b, "Backlog")
	require.NoError(t, err)

	// _archive should not appear as a lane.
	lanes, err := ReadLanes(b.Dir)
	require.NoError(t, err)
	require.Len(t, lanes, 1)
	assert.Equal(t, "backlog", lanes[0].Name)
}

func TestRenameLane(t *testing.T) {
	b := newTestBoard(t)
	_, err := AddLane(b, "Backlog")
	require.NoError(t, err)

	err = RenameLane(b, "Backlog", "To Do")
	require.NoError(t, err)

	lanes, err := ReadLanes(b.Dir)
	require.NoError(t, err)
	require.Len(t, lanes, 1)
	assert.Equal(t, "to do", lanes[0].Name)
}

func TestRenameLane_NotFound(t *testing.T) {
	b := newTestBoard(t)
	err := RenameLane(b, "Nonexistent", "X")
	assert.Error(t, err)
}

func TestRemoveLane_Empty(t *testing.T) {
	b := newTestBoard(t)
	_, err := AddLane(b, "Temp")
	require.NoError(t, err)

	err = RemoveLane(b, "Temp")
	require.NoError(t, err)

	lanes, err := ReadLanes(b.Dir)
	require.NoError(t, err)
	assert.Empty(t, lanes)
}

func TestRemoveLane_NonEmpty(t *testing.T) {
	b := newTestBoard(t)
	lane, err := AddLane(b, "Work")
	require.NoError(t, err)

	// Put a file in the lane to simulate a card.
	f, err := os.Create(filepath.Join(lane.Dir, "001-ab1c2-card.md"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	err = RemoveLane(b, "Work")
	assert.Error(t, err)
}

func TestLaneByName(t *testing.T) {
	b := newTestBoard(t)
	_, err := AddLane(b, "Backlog")
	require.NoError(t, err)

	lanes, err := ReadLanes(b.Dir)
	require.NoError(t, err)

	l, ok := LaneByName(lanes, "BACKLOG")
	require.True(t, ok)
	assert.Equal(t, 1, l.Order)

	_, ok = LaneByName(lanes, "missing")
	assert.False(t, ok)
}
