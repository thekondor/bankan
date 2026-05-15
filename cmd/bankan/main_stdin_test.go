package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/internal/service"
)

// setupTestBoard creates a temporary board with one lane ("Backlog") and
// returns the board directory and lane name.
func setupTestBoard(t *testing.T) (boardDir string, laneName string) {
	t.Helper()
	b, err := bankan.InitBoard(t.TempDir(), "Test Board")
	require.NoError(t, err)
	lane, err := bankan.AddLane(b, "Backlog")
	require.NoError(t, err)
	return b.Dir, lane.Name
}

func listTestCards(t *testing.T, boardDir, laneName string) []*bankan.Card {
	t.Helper()
	reg, id, err := service.NewSingleRegistry(boardDir)
	require.NoError(t, err)
	cards, err := reg.ListCards(id, laneName)
	require.NoError(t, err)
	return cards
}

func listTestComments(t *testing.T, boardDir, cardID string) []bankan.Comment {
	t.Helper()
	reg, id, err := service.NewSingleRegistry(boardDir)
	require.NoError(t, err)
	comments, err := reg.ListComments(id, cardID)
	require.NoError(t, err)
	return comments
}

func addTestCardDirect(t *testing.T, boardDir, laneName, title string) *bankan.Card {
	t.Helper()
	b, err := bankan.ReadBoard(boardDir)
	require.NoError(t, err)
	lanes, err := bankan.ReadLanes(boardDir)
	require.NoError(t, err)
	var lane bankan.Lane
	for _, l := range lanes {
		if l.Name == laneName {
			lane = l
			break
		}
	}
	card, err := bankan.AddCard(b, lane, title, "", nil)
	require.NoError(t, err)
	return card
}

func TestStdinInput_CardAdd(t *testing.T) {
	boardDir, laneName := setupTestBoard(t)

	stdinContent := "# Heading\n\nThis is the **body** from stdin."

	cmd := newCardAddCmd()
	cmd.SetArgs([]string{"--board", boardDir, "--lane", laneName, "--title", "Stdin Card", "--body", "-"})
	cmd.SetIn(strings.NewReader(stdinContent))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	require.NoError(t, cmd.Execute())

	cards := listTestCards(t, boardDir, laneName)
	require.Len(t, cards, 1)
	assert.Equal(t, "Stdin Card", cards[0].Title)
	// The frontmatter serializer appends a trailing newline if the body lacks one.
	assert.Equal(t, stdinContent+"\n", cards[0].Body)
}

func TestStdinInput_CardEdit(t *testing.T) {
	boardDir, laneName := setupTestBoard(t)
	card := addTestCardDirect(t, boardDir, laneName, "Original Title")

	updatedBody := "# Updated\n\nBody from stdin for edit."

	cmd := newCardEditCmd()
	cmd.SetArgs([]string{"--board", boardDir, "--body", "-", card.ID})
	cmd.SetIn(strings.NewReader(updatedBody))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	require.NoError(t, cmd.Execute())

	cards := listTestCards(t, boardDir, laneName)
	require.Len(t, cards, 1)
	// The frontmatter serializer appends a trailing newline if the body lacks one.
	assert.Equal(t, updatedBody+"\n", cards[0].Body)
}

func TestStdinInput_CommentAdd(t *testing.T) {
	boardDir, laneName := setupTestBoard(t)
	card := addTestCardDirect(t, boardDir, laneName, "Card For Comment")

	commentText := "This is a comment from stdin."

	cmd := newCommentAddCmd()
	cmd.SetArgs([]string{"--board", boardDir, "--text", "-", "--author", "tester", card.ID})
	cmd.SetIn(strings.NewReader(commentText))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	require.NoError(t, cmd.Execute())

	comments := listTestComments(t, boardDir, card.ID)
	require.Len(t, comments, 1)
	// AddComment trims the body internally.
	assert.Equal(t, strings.TrimSpace(commentText), comments[0].Body)
}

func TestStdinInput_CommentEdit(t *testing.T) {
	boardDir, laneName := setupTestBoard(t)
	card := addTestCardDirect(t, boardDir, laneName, "Card For Comment Edit")

	comment, err := bankan.AddComment(card.FilePath, "tester", "original comment")
	require.NoError(t, err)

	updatedText := "Updated comment from stdin."

	cmd := newCommentEditCmd()
	cmd.SetArgs([]string{"--board", boardDir, "--card", card.ID, "--text", "-", comment.ID})
	cmd.SetIn(strings.NewReader(updatedText))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	require.NoError(t, cmd.Execute())

	comments := listTestComments(t, boardDir, card.ID)
	require.Len(t, comments, 1)
	// UpdateComment trims the body internally.
	assert.Equal(t, strings.TrimSpace(updatedText), comments[0].Body)
}

func TestStdinInput_NonDash(t *testing.T) {
	boardDir, laneName := setupTestBoard(t)

	// A plain (non-"-") value must NOT read from stdin.
	cmd := newCardAddCmd()
	cmd.SetArgs([]string{"--board", boardDir, "--lane", laneName, "--title", "Normal Card", "--body", "explicit body"})
	cmd.SetIn(strings.NewReader("SHOULD NOT BE READ"))
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	require.NoError(t, cmd.Execute())

	cards := listTestCards(t, boardDir, laneName)
	require.Len(t, cards, 1)
	// The frontmatter serializer appends a trailing newline if the body lacks one.
	assert.Equal(t, "explicit body\n", cards[0].Body)
}
