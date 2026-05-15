package bankan

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommentHeader_Valid(t *testing.T) {
	line := "## a1b2c · 2026-01-01T10:00:00Z · alice"
	id, ts, author, ok := parseCommentHeader(line)
	require.True(t, ok)
	assert.Equal(t, "a1b2c", id)
	assert.Equal(t, "alice", author)
	assert.Equal(t, 2026, ts.Year())
}

func TestParseCommentHeader_Missing(t *testing.T) {
	_, _, _, ok := parseCommentHeader("# Comments: ab12c")
	assert.False(t, ok)
}

func TestParseCommentHeader_BadTimestamp(t *testing.T) {
	_, _, _, ok := parseCommentHeader("## id · not-a-time · author")
	assert.False(t, ok)
}

func TestFormatAndParseHeader_Roundtrip(t *testing.T) {
	c := Comment{
		ID:        "ab12c",
		CreatedAt: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC),
		Author:    "bob",
		Body:      "test",
	}
	line := formatCommentHeader(c)
	id, ts, author, ok := parseCommentHeader(line)
	require.True(t, ok)
	assert.Equal(t, "ab12c", id)
	assert.Equal(t, "bob", author)
	assert.Equal(t, c.CreatedAt, ts)
}

func TestSerializeAndParseComments(t *testing.T) {
	comments := []Comment{
		{
			ID:        "aaaaa",
			CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
			Author:    "alice",
			Body:      "First comment.",
		},
		{
			ID:        "bbbbb",
			CreatedAt: time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC),
			Author:    "bob",
			Body:      "Second comment with **markdown**.",
		},
	}

	content := SerializeComments("card1", comments)

	parsed, err := parseComments(content)
	require.NoError(t, err)
	require.Len(t, parsed, 2)

	assert.Equal(t, "aaaaa", parsed[0].ID)
	assert.Equal(t, "alice", parsed[0].Author)
	assert.Equal(t, "First comment.", parsed[0].Body)

	assert.Equal(t, "bbbbb", parsed[1].ID)
	assert.Equal(t, "bob", parsed[1].Author)
	assert.Equal(t, "Second comment with **markdown**.", parsed[1].Body)
}

func TestParseComments_EmptyFile(t *testing.T) {
	comments, err := parseComments("")
	require.NoError(t, err)
	assert.Empty(t, comments)
}

func TestParseComments_HeaderOnly(t *testing.T) {
	comments, err := parseComments("# Comments: xyz\n")
	require.NoError(t, err)
	assert.Empty(t, comments)
}

func TestAddComment_CreatesAndAppendsFile(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card With Comments")

	comment1, err := AddComment(c.FilePath, "alice", "Hello world.")
	require.NoError(t, err)
	assert.Len(t, comment1.ID, idLength)

	comment2, err := AddComment(c.FilePath, "bob", "Second comment.")
	require.NoError(t, err)

	all, err := ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, comment1.ID, all[0].ID)
	assert.Equal(t, comment2.ID, all[1].ID)
	assert.Equal(t, "Hello world.", all[0].Body)
	assert.Equal(t, "Second comment.", all[1].Body)
}

func TestAddComment_EmptyAuthorError(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	_, err := AddComment(c.FilePath, "", "Body")
	assert.Error(t, err)
}

func TestAddComment_EmptyBodyError(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	_, err := AddComment(c.FilePath, "alice", "")
	assert.Error(t, err)
}

func TestReadComments_FileNotExist(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "No Comments")

	// No comments file created yet — must return empty slice, no error.
	comments, err := ReadComments(c.FilePath)
	require.NoError(t, err)
	assert.Empty(t, comments)
}

func TestUpdateComment_UpdatesBody(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	cm1, err := AddComment(c.FilePath, "alice", "Original body.")
	require.NoError(t, err)
	_, err = AddComment(c.FilePath, "bob", "Other comment.")
	require.NoError(t, err)

	updated, err := UpdateComment(c.FilePath, cm1.ID, "Edited body.")
	require.NoError(t, err)
	assert.Equal(t, cm1.ID, updated.ID)
	assert.Equal(t, "alice", updated.Author)
	assert.Equal(t, "Edited body.", updated.Body)

	all, err := ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "Edited body.", all[0].Body)
	assert.Equal(t, "Other comment.", all[1].Body)
}

func TestUpdateComment_PreservesOtherComments(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")

	_, err := AddComment(c.FilePath, "alice", "First.")
	require.NoError(t, err)
	cm2, err := AddComment(c.FilePath, "bob", "Second.")
	require.NoError(t, err)
	_, err = AddComment(c.FilePath, "carol", "Third.")
	require.NoError(t, err)

	_, err = UpdateComment(c.FilePath, cm2.ID, "Updated second.")
	require.NoError(t, err)

	all, err := ReadComments(c.FilePath)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "First.", all[0].Body)
	assert.Equal(t, "Updated second.", all[1].Body)
	assert.Equal(t, "Third.", all[2].Body)
}

func TestUpdateComment_NotFound(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")
	_, _ = AddComment(c.FilePath, "alice", "Some comment.")

	_, err := UpdateComment(c.FilePath, "zzzzz", "New body.")
	assert.Error(t, err)
}

func TestUpdateComment_EmptyBody(t *testing.T) {
	b, lane := boardWithLane(t)
	c := addTestCard(t, b, lane, "Card")
	cm, err := AddComment(c.FilePath, "alice", "Original.")
	require.NoError(t, err)

	_, err = UpdateComment(c.FilePath, cm.ID, "")
	assert.Error(t, err)
}
