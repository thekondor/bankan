package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderBoardHeaderHTML(t *testing.T, data BoardPageData) string {
	t.Helper()
	var buf bytes.Buffer
	err := boardHeader(data).Render(context.Background(), &buf)
	require.NoError(t, err)
	return buf.String()
}

func minimalBoardHeaderData() BoardPageData {
	return BoardPageData{
		CurrentBoard:     BoardData{ID: "b1", Name: "Board 1"},
		CurrentWorkspace: WorkspaceData{ID: "ws1", Name: "WS1"},
		AllBoards:        []BoardData{{ID: "b1", Name: "Board 1"}},
	}
}

// No hidden boards, no archived views: placeholder rendered, no section titles.
func TestBoardHeader_DropdownPanel_NoHiddenNoArchived(t *testing.T) {
	html := renderBoardHeaderHTML(t, minimalBoardHeaderData())

	assert.Contains(t, html, `overflow-panel-empty`)
	assert.Contains(t, html, "No hidden boards")
	assert.NotContains(t, html, `archived-views-section-title`)
	assert.NotContains(t, html, `data-hidden-item-id`)
}

// Hidden boards only: placeholder absent, hidden items present, no archived section.
func TestBoardHeader_DropdownPanel_HiddenOnly(t *testing.T) {
	data := minimalBoardHeaderData()
	data.HiddenBoards = []BoardData{
		{ID: "h1", Name: "Hidden Board 1"},
		{ID: "h2", Name: "Hidden Board 2"},
	}
	html := renderBoardHeaderHTML(t, data)

	assert.NotContains(t, html, `overflow-panel-empty`)
	assert.Contains(t, html, `data-hidden-item-id="h1"`)
	assert.Contains(t, html, `data-hidden-item-id="h2"`)
	assert.NotContains(t, html, `archived-views-section-title`)
}

// Archived views only: placeholder rendered (JS hides it at runtime when items are
// visible in the dropdown); archived section title and items present.
func TestBoardHeader_DropdownPanel_ArchivedOnly(t *testing.T) {
	data := minimalBoardHeaderData()
	data.ArchivedViewBoards = []BoardData{
		{ID: "a1", Name: "Archived View 1"},
	}
	html := renderBoardHeaderHTML(t, data)

	// Server always renders the placeholder when HiddenBoards is empty;
	// JS (_syncArchivedDropdownItems) hides it at runtime when archived items are visible.
	assert.Contains(t, html, `overflow-panel-empty`)
	assert.Contains(t, html, `archived-views-section-title`)
	assert.Contains(t, html, `data-archived-item-id="a1"`)
	assert.NotContains(t, html, `data-hidden-item-id`)
}

// Both hidden and archived: placeholder absent, both sections present.
func TestBoardHeader_DropdownPanel_BothHiddenAndArchived(t *testing.T) {
	data := minimalBoardHeaderData()
	data.HiddenBoards = []BoardData{{ID: "h1", Name: "Hidden Board 1"}}
	data.ArchivedViewBoards = []BoardData{{ID: "a1", Name: "Archived View 1"}}
	html := renderBoardHeaderHTML(t, data)

	assert.NotContains(t, html, `overflow-panel-empty`)
	assert.Contains(t, html, `data-hidden-item-id="h1"`)
	assert.Contains(t, html, `archived-views-section-title`)
	assert.Contains(t, html, `data-archived-item-id="a1"`)
}

// The current board must not appear as a dropdown item in the archived section.
func TestBoardHeader_DropdownPanel_CurrentBoardExcludedFromArchived(t *testing.T) {
	data := minimalBoardHeaderData()
	data.CurrentBoard = BoardData{ID: "a1", Name: "Archived View 1", IsArchived: true}
	data.ArchivedViewBoards = []BoardData{
		{ID: "a1", Name: "Archived View 1"},
		{ID: "a2", Name: "Archived View 2"},
	}
	html := renderBoardHeaderHTML(t, data)

	assert.NotContains(t, html, `data-archived-item-id="a1"`,
		"current board should be excluded from the archived dropdown items")
	assert.Contains(t, html, `data-archived-item-id="a2"`)
}

// Archived tab string for the current board must appear in the tab bar (not the dropdown).
func TestBoardHeader_ArchivedTabBar_CurrentBoardShown(t *testing.T) {
	data := minimalBoardHeaderData()
	data.CurrentBoard = BoardData{ID: "a1", Name: "Active Archived", IsArchived: true}
	data.ArchivedViewBoards = []BoardData{
		{ID: "a1", Name: "Active Archived"},
	}
	html := renderBoardHeaderHTML(t, data)

	assert.True(t, strings.Contains(html, `data-archived-id="a1"`),
		"archived tab should exist in the tab bar")
	assert.True(t, strings.Contains(html, `board-tab-archived`))
}
