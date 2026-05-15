package bankan

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitBoard_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "Test Board")
	require.NoError(t, err)

	assert.Equal(t, "Test Board", b.Name)
	assert.Equal(t, dir, b.Dir)
	assert.WithinDuration(t, time.Now(), b.CreatedAt, 5*time.Second)

	// board.md exists
	_, err = os.Stat(filepath.Join(dir, boardFileName))
	assert.NoError(t, err)

	// _archive dir exists
	_, err = os.Stat(filepath.Join(dir, "_archive"))
	assert.NoError(t, err)
}

func TestInitBoard_ErrorIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	_, err := InitBoard(dir, "First")
	require.NoError(t, err)

	_, err = InitBoard(dir, "Second")
	assert.Error(t, err)
}

func TestReadBoard_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "My Board")
	require.NoError(t, err)

	b.Body = "Some description."
	b.Labels = []Label{{ID: "x1", Name: "Bug", Color: "#ff0000"}}
	require.NoError(t, WriteBoard(b))

	b2, err := ReadBoard(dir)
	require.NoError(t, err)

	assert.Equal(t, "My Board", b2.Name)
	assert.Equal(t, "Some description.\n", b2.Body)
	require.Len(t, b2.Labels, 1)
	assert.Equal(t, "Bug", b2.Labels[0].Name)
}

func TestIsBoard(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsBoard(dir))

	_, err := InitBoard(dir, "X")
	require.NoError(t, err)
	assert.True(t, IsBoard(dir))
}

func TestFindBoard_WalksUp(t *testing.T) {
	root := t.TempDir()
	_, err := InitBoard(root, "Root Board")
	require.NoError(t, err)

	nested := filepath.Join(root, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	b, err := FindBoard(nested)
	require.NoError(t, err)
	assert.Equal(t, "Root Board", b.Name)
}

func TestFindBoard_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindBoard(dir)
	assert.Error(t, err)
}

func TestAddLabel(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "X")
	require.NoError(t, err)

	err = AddLabel(b, Label{ID: "l1", Name: "Bug", Color: "#ff0000"})
	require.NoError(t, err)
	assert.Len(t, b.Labels, 1)

	// Reload and verify persisted.
	b2, err := ReadBoard(dir)
	require.NoError(t, err)
	require.Len(t, b2.Labels, 1)
	assert.Equal(t, "l1", b2.Labels[0].ID)
}

func TestAddLabel_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "X")
	require.NoError(t, err)

	require.NoError(t, AddLabel(b, Label{ID: "l1", Name: "Bug", Color: "#ff0000"}))
	err = AddLabel(b, Label{ID: "l1", Name: "Other", Color: "#00ff00"})
	assert.Error(t, err)
}

func TestUpdateLabel(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "X")
	require.NoError(t, err)

	require.NoError(t, AddLabel(b, Label{ID: "l1", Name: "Bug", Color: "#ff0000"}))
	require.NoError(t, UpdateLabel(b, Label{ID: "l1", Name: "Defect", Color: "#aa0000"}))

	b2, err := ReadBoard(dir)
	require.NoError(t, err)
	assert.Equal(t, "Defect", b2.Labels[0].Name)
}

func TestRemoveLabel(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "X")
	require.NoError(t, err)

	require.NoError(t, AddLabel(b, Label{ID: "l1", Name: "Bug", Color: "#ff0000"}))
	require.NoError(t, RemoveLabel(b, "l1"))
	assert.Empty(t, b.Labels)

	err = RemoveLabel(b, "missing")
	assert.Error(t, err)
}

func TestHideBoard_ShowBoard(t *testing.T) {
	dir := t.TempDir()
	b, err := InitBoard(dir, "X")
	require.NoError(t, err)
	assert.False(t, b.Hidden)

	require.NoError(t, HideBoard(b))
	assert.True(t, b.Hidden)
	got, err := ReadBoard(dir)
	require.NoError(t, err)
	assert.True(t, got.Hidden)

	require.NoError(t, ShowBoard(b))
	assert.False(t, b.Hidden)
	got, err = ReadBoard(dir)
	require.NoError(t, err)
	assert.False(t, got.Hidden)
}
