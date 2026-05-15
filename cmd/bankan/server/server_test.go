package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/cmd/bankan/server"
	"github.com/thekondor/bankan/internal/service"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

type testClient struct {
	srv   *httptest.Server
	token string
}

func newTestServer(t *testing.T) *testClient {
	t.Helper()
	dir := t.TempDir()
	_, err := bankan.InitBoard(dir, "Test Board")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{dir}, t.TempDir())
	require.NoError(t, err)

	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "test-token"}, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &testClient{srv: ts, token: "test-token"}
}

func newTestServerMultiBoard(t *testing.T) (*testClient, string, string) {
	t.Helper()
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	_, err := bankan.InitBoard(dir1, "Board One")
	require.NoError(t, err)
	_, err = bankan.InitBoard(dir2, "Board Two")
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{dir1, dir2}, "")
	require.NoError(t, err)

	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "test-token"}, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &testClient{srv: ts, token: "test-token"}, filepath.Base(dir1), filepath.Base(dir2)
}

func (c *testClient) do(method, path string, body any, token string) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Bankan-Token", token)
	}
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

func (c *testClient) get(path string) *http.Response {
	return c.do("GET", path, nil, "")
}
func (c *testClient) post(path string, body any) *http.Response {
	return c.do("POST", path, body, c.token)
}
func (c *testClient) patch(path string, body any) *http.Response {
	return c.do("PATCH", path, body, c.token)
}
func (c *testClient) del(path string) *http.Response {
	return c.do("DELETE", path, nil, c.token)
}

func boardID(t *testing.T, c *testClient) string {
	t.Helper()
	resp := c.get("/api/v1/boards")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var boards []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&boards))
	require.Len(t, boards, 1)
	return boards[0]["id"].(string)
}

func decodeJSON(t *testing.T, r io.Reader, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(r).Decode(v))
}

// ─── Token protection ─────────────────────────────────────────────────────────

func TestServer_TokenRequired_MissingToken(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp := c.do("POST", "/api/v1/boards/"+id+"/lanes", map[string]any{"name": "test"}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServer_TokenRequired_WrongToken(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp := c.do("POST", "/api/v1/boards/"+id+"/lanes", map[string]any{"name": "test"}, "wrong-token")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServer_GETRequiresNoToken(t *testing.T) {
	c := newTestServer(t)
	resp := c.get("/api/v1/boards")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_NoTokenMode(t *testing.T) {
	dir := t.TempDir()
	_, err := bankan.InitBoard(dir, "Board")
	require.NoError(t, err)
	reg, err := service.NewRegistry([]string{dir}, "")
	require.NoError(t, err)
	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{NoToken: true}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Mutating request without token should succeed
	resp, _ := http.Post(ts.URL+"/api/v1/boards/"+filepath.Base(dir)+"/lanes",
		"application/json", bytes.NewBufferString(`{"name":"todo"}`))
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// ─── Board API ────────────────────────────────────────────────────────────────

func TestAPI_ListBoards(t *testing.T) {
	c := newTestServer(t)
	resp := c.get("/api/v1/boards")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var boards []map[string]any
	decodeJSON(t, resp.Body, &boards)
	require.Len(t, boards, 1)
	assert.Equal(t, "Test Board", boards[0]["name"])
	assert.Equal(t, false, boards[0]["is_view"])
}

func TestAPI_GetBoard(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp := c.get("/api/v1/boards/" + id)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var b map[string]any
	decodeJSON(t, resp.Body, &b)
	assert.Equal(t, "Test Board", b["name"])
}

func TestAPI_GetBoard_NotFound(t *testing.T) {
	c := newTestServer(t)
	resp := c.get("/api/v1/boards/nonexistent")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAPI_InitBoard_WithRootDir(t *testing.T) {
	rootDir := t.TempDir()
	reg, err := service.NewRegistry([]string{}, rootDir)
	require.NoError(t, err)
	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/boards",
		bytes.NewBufferString(`{"name":"new-board"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bankan-Token", "tok")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var b map[string]any
	decodeJSON(t, resp.Body, &b)
	assert.Equal(t, "new-board", b["name"])
}

func TestAPI_InitBoard_NoRootDir_Forbidden(t *testing.T) {
	c := newTestServer(t)
	// The test server uses t.TempDir() as rootDir, so it actually has one.
	// Let's test with an explicit no-root server.
	dir := t.TempDir()
	_, err := bankan.InitBoard(dir, "Board")
	require.NoError(t, err)
	reg, err := service.NewRegistry([]string{dir}, "")
	require.NoError(t, err)
	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/boards",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bankan-Token", "tok")
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	_ = c // suppress unused warning
}

// ─── Lane API ─────────────────────────────────────────────────────────────────

func TestAPI_LaneCRUD(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Add lane
	resp := c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "Backlog"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var lane map[string]any
	decodeJSON(t, resp.Body, &lane)
	assert.Equal(t, "backlog", lane["name"])

	// List lanes
	resp = c.get("/api/v1/boards/" + id + "/lanes")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var lanes []map[string]any
	decodeJSON(t, resp.Body, &lanes)
	assert.Len(t, lanes, 1)

	// Rename lane
	resp = c.patch("/api/v1/boards/"+id+"/lanes/backlog", map[string]any{"new_name": "Todo"})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify rename
	resp = c.get("/api/v1/boards/" + id + "/lanes")
	var renamed []map[string]any
	decodeJSON(t, resp.Body, &renamed)
	assert.Equal(t, "todo", renamed[0]["name"])

	// Remove lane
	resp = c.del("/api/v1/boards/" + id + "/lanes/todo")
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAPI_AddLane_Missing_Name(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp := c.post("/api/v1/boards/"+id+"/lanes", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ─── Card API ─────────────────────────────────────────────────────────────────

func TestAPI_CardCRUD(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Setup lane
	resp := c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "Todo"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Add card
	resp = c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Fix bug", "body": "Some body",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	assert.Equal(t, "Fix bug", card["title"])
	cardID := card["id"].(string)

	// Get card
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var gotCard map[string]any
	decodeJSON(t, resp.Body, &gotCard)
	assert.Equal(t, "Fix bug", gotCard["title"])

	// Update card
	title := "Fixed bug"
	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID), map[string]any{
		"title": title,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decodeJSON(t, resp.Body, &updated)
	assert.Equal(t, "Fixed bug", updated["title"])

	// List cards in lane
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards?lane=todo", id))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var cards []map[string]any
	decodeJSON(t, resp.Body, &cards)
	assert.Len(t, cards, 1)

	// Delete card
	resp = c.do("DELETE", fmt.Sprintf("/api/v1/boards/%s/cards/%s?force=true", id, cardID), nil, c.token)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAPI_DeleteCard_RequiresForce(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.del(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_MoveCard(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "done"})

	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/move", id, cardID), map[string]any{"to_lane": "done"})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	moved := c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID))
	var movedCard map[string]any
	decodeJSON(t, moved.Body, &movedCard)
	assert.Equal(t, "done", movedCard["lane"])
}

func TestAPI_ArchiveAndRestoreCard(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Archive me"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	// Archive
	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/archive", id, cardID), nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// List archived
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards?archived=true", id))
	var archived []map[string]any
	decodeJSON(t, resp.Body, &archived)
	require.Len(t, archived, 1)
	assert.Equal(t, cardID, archived[0]["id"])

	// Restore
	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/restore", id, cardID), map[string]any{"to_lane": "todo"})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// ─── Comment API ──────────────────────────────────────────────────────────────

func TestAPI_Comments(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	// Add comment
	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice",
		"body":   "Great card!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var cm map[string]any
	decodeJSON(t, resp.Body, &cm)
	assert.Equal(t, "alice", cm["author"])
	assert.Equal(t, "Great card!", cm["body"])

	// List comments
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var comments []map[string]any
	decodeJSON(t, resp.Body, &comments)
	require.Len(t, comments, 1)
}

func TestAPI_AddComment_EmptyBody(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice", "body": "",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_UpdateComment(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice", "body": "Original body",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var cm map[string]any
	decodeJSON(t, resp.Body, &cm)
	commentID := cm["id"].(string)

	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments/%s", id, cardID, commentID), map[string]any{
		"body": "Edited body",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decodeJSON(t, resp.Body, &updated)
	assert.Equal(t, commentID, updated["id"])
	assert.Equal(t, "alice", updated["author"])
	assert.Equal(t, "Edited body", updated["body"])

	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID))
	var comments []map[string]any
	decodeJSON(t, resp.Body, &comments)
	require.Len(t, comments, 1)
	assert.Equal(t, "Edited body", comments[0]["body"])
}

func TestAPI_UpdateComment_NotFound(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)
	c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice", "body": "Some comment",
	})

	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments/zzzzz", id, cardID), map[string]any{
		"body": "New body",
	})
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestAPI_UpdateComment_EmptyBody(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{"lane": "todo", "title": "Card"})
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice", "body": "Some comment",
	})
	var cm map[string]any
	decodeJSON(t, resp.Body, &cm)
	commentID := cm["id"].(string)

	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments/%s", id, cardID, commentID), map[string]any{
		"body": "",
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ─── Label API ────────────────────────────────────────────────────────────────

func TestAPI_LabelCRUD(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Add label
	resp := c.post("/api/v1/boards/"+id+"/labels", map[string]any{"name": "Bug", "color": "#ef4444"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var lbl map[string]any
	decodeJSON(t, resp.Body, &lbl)
	assert.Equal(t, "Bug", lbl["name"])
	labelID := lbl["id"].(string)

	// List labels
	resp = c.get("/api/v1/boards/" + id + "/labels")
	var labels []map[string]any
	decodeJSON(t, resp.Body, &labels)
	assert.Len(t, labels, 1)

	// Update label
	newName := "Defect"
	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/labels/%s", id, labelID), map[string]any{"name": &newName})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decodeJSON(t, resp.Body, &updated)
	assert.Equal(t, "Defect", updated["name"])

	// Remove label
	resp = c.del(fmt.Sprintf("/api/v1/boards/%s/labels/%s", id, labelID))
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAPI_AddLabel_MissingFields(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp := c.post("/api/v1/boards/"+id+"/labels", map[string]any{"name": "bug"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ─── Multi-board ──────────────────────────────────────────────────────────────

func TestAPI_MultiBoard_IndependentLanes(t *testing.T) {
	c, id1, id2 := newTestServerMultiBoard(t)

	c.post("/api/v1/boards/"+id1+"/lanes", map[string]any{"name": "todo"})

	// Board2 should have no lanes
	resp := c.get("/api/v1/boards/" + id2 + "/lanes")
	var lanes []map[string]any
	decodeJSON(t, resp.Body, &lanes)
	assert.Empty(t, lanes)
}

// ─── View board API ───────────────────────────────────────────────────────────

func TestAPI_ViewBoard_SyncAndArchive(t *testing.T) {
	parentDir := t.TempDir()
	viewDir   := t.TempDir()
	_, err := bankan.InitBoard(parentDir, "Parent")
	require.NoError(t, err)

	// Add lane + label to parent
	parentReg, parentID, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	_, err = parentReg.AddLane(parentID, "backlog")
	require.NoError(t, err)
	lbl, err := parentReg.AddLabel(parentID, "feature", "#3b82f6")
	require.NoError(t, err)

	_, err = bankan.InitViewBoard(viewDir, "Sprint", parentDir, lbl.ID)
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{parentDir, viewDir}, "")
	require.NoError(t, err)

	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	viewID := filepath.Base(viewDir)

	doReq := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", "tok")
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}

	// Get view board
	resp := doReq("GET", "/api/v1/boards/"+viewID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var vb map[string]any
	decodeJSON(t, resp.Body, &vb)
	assert.Equal(t, true, vb["is_view"])

	// Sync
	resp = doReq("POST", "/api/v1/boards/"+viewID+"/sync", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Archive view board
	resp = doReq("POST", "/api/v1/boards/"+viewID+"/archive", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Get again - should be archived
	resp = doReq("GET", "/api/v1/boards/"+viewID, nil)
	decodeJSON(t, resp.Body, &vb)
	assert.NotNil(t, vb["archived_at"])
}

// ─── UI endpoints ─────────────────────────────────────────────────────────────

func TestUI_Root_Redirect(t *testing.T) {
	c := newTestServer(t)
	resp, _ := http.Get(c.srv.URL + "/")
	// Should redirect to /ui/boards/{id}
	assert.Equal(t, http.StatusOK, resp.StatusCode) // follows redirect
	assert.Contains(t, resp.Request.URL.Path, "/ui/boards/")
}

func TestUI_Board_Page(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	resp, _ := http.Get(c.srv.URL + "/ui/boards/" + id)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "bankan")
	assert.Contains(t, string(body), "Test Board")
}

func TestUI_Static_CSS(t *testing.T) {
	c := newTestServer(t)
	resp, _ := http.Get(c.srv.URL + "/static/style.css")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	ct := resp.Header.Get("Content-Type")
	assert.Contains(t, ct, "css")
}

// ─── Full lifecycle integration ───────────────────────────────────────────────

func TestAPI_FullLifecycle(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// 1. Add two lanes
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "done"})

	// 2. Add a label
	resp := c.post("/api/v1/boards/"+id+"/labels", map[string]any{"name": "feature", "color": "#3b82f6"})
	var lbl map[string]any
	decodeJSON(t, resp.Body, &lbl)
	labelID := lbl["id"].(string)

	// 3. Add a card with the label
	resp = c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Build REST API", "body": "Implementation", "label_ids": []string{labelID},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)
	assert.Contains(t, card["labels"], labelID)

	// 4. Add a comment
	c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "dev", "body": "Implemented!",
	})

	// 5. Move to done
	c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/move", id, cardID), map[string]any{"to_lane": "done"})

	// 6. Verify card is in done
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID))
	decodeJSON(t, resp.Body, &card)
	assert.Equal(t, "done", card["lane"])

	// 7. Archive card
	c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/archive", id, cardID), nil)

	// 8. Verify archived
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards?archived=true", id))
	var archived []map[string]any
	decodeJSON(t, resp.Body, &archived)
	require.Len(t, archived, 1)
	assert.NotNil(t, archived[0]["archived_at"])

	// 9. Restore
	c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/restore", id, cardID), map[string]any{"to_lane": "todo"})

	// 10. Delete permanently
	c.do("DELETE", fmt.Sprintf("/api/v1/boards/%s/cards/%s?force=true", id, cardID), nil, c.token)

	// 11. Verify gone
	resp = c.get(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── InitViewBoard API ────────────────────────────────────────────────────────

func newTestServerWithParentBoard(t *testing.T) (*testClient, string, bankan.Label) {
	t.Helper()
	rootDir := t.TempDir()

	parentDir := filepath.Join(rootDir, "parent")
	require.NoError(t, os.MkdirAll(parentDir, 0o755))
	_, err := bankan.InitBoard(parentDir, "Parent Board")
	require.NoError(t, err)

	parentID := filepath.Base(parentDir)

	// We need a label on the parent for view board creation; use a temporary
	// single-board registry to add it, then re-open the full registry.
	tmpReg, _, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	lbl, err := tmpReg.AddLabel(parentID, "feature", "#3b82f6")
	require.NoError(t, err)

	// Re-scan so the main registry picks up the label written to disk.
	reg, err := service.NewRegistry([]string{parentDir}, rootDir)
	require.NoError(t, err)

	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &testClient{srv: ts, token: "tok"}, parentID, lbl
}

func TestAPI_InitViewBoard_WithRootDir(t *testing.T) {
	c, parentID, lbl := newTestServerWithParentBoard(t)

	resp := c.post("/api/v1/view-boards", map[string]any{
		"name":            "Sprint One",
		"parent_id":       parentID,
		"filter_label_id": lbl.ID,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]any
	decodeJSON(t, resp.Body, &result)

	// ID must be the slugified directory name.
	assert.Equal(t, "sprint-one", result["id"])
	assert.Equal(t, "Sprint One", result["name"])
	assert.Equal(t, true, result["is_view"])
	assert.Equal(t, lbl.ID, result["filter_label_id"])

	// The new view board must appear in the board list.
	listResp := c.get("/api/v1/boards")
	var boards []map[string]any
	decodeJSON(t, listResp.Body, &boards)
	ids := make([]string, len(boards))
	for i, b := range boards {
		ids[i] = b["id"].(string)
	}
	assert.Contains(t, ids, "sprint-one")
}

func TestAPI_InitViewBoard_NoRootDir_Forbidden(t *testing.T) {
	// Server with no rootDir cannot create view boards.
	parentDir := t.TempDir()
	_, err := bankan.InitBoard(parentDir, "Parent")
	require.NoError(t, err)
	reg, err := service.NewRegistry([]string{parentDir}, "")
	require.NoError(t, err)
	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/view-boards",
		bytes.NewBufferString(`{"name":"x","parent_id":"parent","filter_label_id":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bankan-Token", "tok")
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestUI_SyncViewBoard(t *testing.T) {
	parentDir := t.TempDir()
	viewDir   := t.TempDir()

	_, err := bankan.InitBoard(parentDir, "Parent")
	require.NoError(t, err)

	parentReg, parentID, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	_, err = parentReg.AddLane(parentID, "backlog")
	require.NoError(t, err)
	lbl, err := parentReg.AddLabel(parentID, "feature", "#3b82f6")
	require.NoError(t, err)

	_, err = bankan.InitViewBoard(viewDir, "Sprint", parentDir, lbl.ID)
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{parentDir, viewDir}, "")
	require.NoError(t, err)

	// Add a card with the filter label to parent so Sync has something to pick up.
	_, err = parentReg.AddCard(parentID, "backlog", "Task One", "body", []string{lbl.ID})
	require.NoError(t, err)

	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	viewID := filepath.Base(viewDir)

	// Sync via UI endpoint — expect HTML back (board view fragment).
	req, _ := http.NewRequest("POST", ts.URL+"/ui/boards/"+viewID+"/sync", nil)
	req.Header.Set("X-Bankan-Token", "tok")
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	ct := resp.Header.Get("Content-Type")
	assert.Contains(t, ct, "text/html")

	// After sync, the card should now be visible in the view board.
	cardsResp, _ := http.DefaultClient.Do(func() *http.Request {
		r, _ := http.NewRequest("GET", ts.URL+"/api/v1/boards/"+viewID+"/cards", nil)
		return r
	}())
	var cards []map[string]any
	decodeJSON(t, cardsResp.Body, &cards)
	require.Len(t, cards, 1)
	assert.Equal(t, "Task One", cards[0]["title"])
}

// ─── UI card permalink ────────────────────────────────────────────────────────

// TestUI_CardModal_PermalinkButtons verifies that the card detail modal HTML
// contains the copyCardLink() and closeModal() call sites introduced to support
// shareable card links.
func TestUI_CardModal_PermalinkButtons(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Create a lane and a card.
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Permalink card", "body": "body text",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	// Fetch the card modal HTML.
	modalResp := c.get(fmt.Sprintf("/ui/modals/card/%s/boards/%s", cardID, id))
	require.Equal(t, http.StatusOK, modalResp.StatusCode)
	body, _ := io.ReadAll(modalResp.Body)
	html := string(body)

	assert.Contains(t, html, "copyCardLink()", "card modal must have a copy-link button")
	assert.Contains(t, html, "closeModal()", "card modal close button must call closeModal()")
}

// TestUI_CardModal_CommentPermalinkButton verifies that each comment rendered
// inside the card detail modal contains a copyCommentLink() call so users can
// copy a direct link to that comment.
func TestUI_CardModal_CommentPermalinkButton(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Create a lane, a card, and a comment.
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Card with comment",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var card map[string]any
	decodeJSON(t, resp.Body, &card)
	cardID := card["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", id, cardID), map[string]any{
		"author": "alice", "body": "Hello world",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var cm map[string]any
	decodeJSON(t, resp.Body, &cm)
	commentID := cm["id"].(string)

	// Fetch the card modal HTML.
	modalResp := c.get(fmt.Sprintf("/ui/modals/card/%s/boards/%s", cardID, id))
	require.Equal(t, http.StatusOK, modalResp.StatusCode)
	body, _ := io.ReadAll(modalResp.Body)
	html := string(body)

	assert.Contains(t, html, fmt.Sprintf("copyCommentLink('%s')", commentID),
		"each comment must render a copyCommentLink() button with its ID")
}

// TestUI_CardModal_CardRemovedFromView verifies that opening a card permalink
// from a view board after the card stub has been removed from that view (but
// the card still exists on the parent board) returns a 200 HTML modal with a
// link pointing to the card on the parent board, instead of a bare 404.
func TestUI_CardModal_CardRemovedFromView(t *testing.T) {
	parentDir := t.TempDir()
	viewDir   := t.TempDir()

	_, err := bankan.InitBoard(parentDir, "Parent Board")
	require.NoError(t, err)

	// Set up the parent board: lane + label + card via the service layer.
	parentReg, parentID, err := service.NewSingleRegistry(parentDir)
	require.NoError(t, err)
	_, err = parentReg.AddLane(parentID, "backlog")
	require.NoError(t, err)
	lbl, err := parentReg.AddLabel(parentID, "sprint", "#3b82f6")
	require.NoError(t, err)
	card, err := parentReg.AddCard(parentID, "backlog", "Important Task", "", []string{lbl.ID})
	require.NoError(t, err)
	cardID := card.ID

	// Create view board filtered by the label and sync it so the stub appears.
	_, err = bankan.InitViewBoard(viewDir, "Sprint View", parentDir, lbl.ID)
	require.NoError(t, err)

	reg, err := service.NewRegistry([]string{parentDir, viewDir}, "")
	require.NoError(t, err)

	viewID := filepath.Base(viewDir)
	err = reg.SyncViewBoard(viewID)
	require.NoError(t, err)

	// Remove the card from the view board (archive it on the view side).
	err = reg.ArchiveCard(viewID, cardID)
	require.NoError(t, err)

	// Start the HTTP server.
	logger := log.New(io.Discard, "", 0)
	srv, err := server.New(reg, server.Config{Token: "tok"}, logger)
	require.NoError(t, err)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Fetch the card modal on the view board — stub is gone, card lives on parent.
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/ui/modals/card/%s/boards/%s", ts.URL, cardID, viewID), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	// Must return 200 with HTML, not 404 JSON.
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// The modal must name the parent board and link to the card there.
	assert.Contains(t, html, "Parent Board", "modal must mention the parent board name")
	assert.Contains(t, html, "/ui/boards/"+parentID+"?card="+cardID,
		"modal must contain a direct link to the card on the parent board")
}

func TestAPI_CardPrimaryLabel(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	// Setup
	resp := c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "Todo"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp = c.post("/api/v1/boards/"+id+"/labels", map[string]any{"name": "Feature", "color": "#3b82f6"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var labelResp map[string]any
	decodeJSON(t, resp.Body, &labelResp)
	labelID := labelResp["id"].(string)

	resp = c.post("/api/v1/boards/"+id+"/labels", map[string]any{"name": "Bug", "color": "#ef4444"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var bugResp map[string]any
	decodeJSON(t, resp.Body, &bugResp)
	bugID := bugResp["id"].(string)

	// Card starts with only Bug as a regular label; Feature is not yet assigned.
	resp = c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Card", "label_ids": []string{bugID},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var cardResp map[string]any
	decodeJSON(t, resp.Body, &cardResp)
	cardID := cardResp["id"].(string)

	// Set Feature as primary label — must also be added to regular labels.
	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID), map[string]any{
		"primary_label": labelID,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updated map[string]any
	decodeJSON(t, resp.Body, &updated)
	assert.Equal(t, labelID, updated["primary_label"], "primary_label must be set")
	labels := toStringSlice(updated["labels"].([]any))
	assert.Contains(t, labels, labelID, "primary label must be added to regular labels")
	assert.Contains(t, labels, bugID, "other labels must remain")

	// Clear primary label
	empty := ""
	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID), map[string]any{
		"primary_label": empty,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var cleared map[string]any
	decodeJSON(t, resp.Body, &cleared)
	assert.Empty(t, cleared["primary_label"], "primary label must be cleared")

	// Setting an invalid label ID must return 404
	resp = c.patch(fmt.Sprintf("/api/v1/boards/%s/cards/%s", id, cardID), map[string]any{
		"primary_label": "zzzzz",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func toStringSlice(in []any) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.(string)
	}
	return out
}

// ─── DuplicateCard API ────────────────────────────────────────────────────────

func TestAPI_DuplicateCard(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)
	c.post("/api/v1/boards/"+id+"/lanes", map[string]any{"name": "todo"})
	resp := c.post("/api/v1/boards/"+id+"/cards", map[string]any{
		"lane": "todo", "title": "Original", "body": "some body",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var src map[string]any
	decodeJSON(t, resp.Body, &src)
	srcID := src["id"].(string)

	resp = c.post(fmt.Sprintf("/api/v1/boards/%s/cards/%s/duplicate", id, srcID), nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var dup map[string]any
	decodeJSON(t, resp.Body, &dup)
	assert.Equal(t, "[dup] Original", dup["title"])
	assert.Equal(t, "some body\n", dup["body"]) // body roundtrips through disk; serialize adds trailing newline
	assert.Equal(t, "todo", dup["lane"])
	assert.NotEqual(t, srcID, dup["id"])
}

func TestAPI_DuplicateCard_NotFound(t *testing.T) {
	c := newTestServer(t)
	id := boardID(t, c)

	resp := c.post(fmt.Sprintf("/api/v1/boards/%s/cards/nope0/duplicate", id), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAPI_ReorderBoards(t *testing.T) {
	c, id1, id2 := newTestServerMultiBoard(t)

	resp := c.post("/api/v1/boards/reorder", map[string]any{"ids": []string{id2, id1}})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify new order via list endpoint.
	resp = c.get("/api/v1/boards")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var boards []map[string]any
	decodeJSON(t, resp.Body, &boards)
	require.Len(t, boards, 2)
	assert.Equal(t, id2, boards[0]["id"])
	assert.Equal(t, id1, boards[1]["id"])
}

func TestAPI_ReorderBoards_NotFound(t *testing.T) {
	c := newTestServer(t)
	resp := c.post("/api/v1/boards/reorder", map[string]any{"ids": []string{"does-not-exist"}})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAPI_ReorderBoards_EmptyIDs(t *testing.T) {
	c := newTestServer(t)
	resp := c.post("/api/v1/boards/reorder", map[string]any{"ids": []string{}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPI_HideBoard_ShowBoard(t *testing.T) {
	c, id1, id2 := newTestServerMultiBoard(t)

	// Hide board1 — not the active board (no HX-Current-URL header).
	resp := c.post("/api/v1/boards/"+id1+"/hide", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Board still listed but hidden field should be set (checked via build).
	resp = c.post("/api/v1/boards/"+id1+"/show", nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Hide unknown board.
	resp = c.post("/api/v1/boards/nope0/hide", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	_ = id2 // referenced to avoid unused error
}

func TestUI_HideBoard_NonActive_Returns204(t *testing.T) {
	c, id1, id2 := newTestServerMultiBoard(t)

	// POST /ui/boards/{id}/hide without HX-Current-URL → 204
	req, _ := http.NewRequest("POST", c.srv.URL+"/ui/boards/"+id1+"/hide", nil)
	req.Header.Set("X-Bankan-Token", c.token)
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	_ = id2
}

func TestUI_HideBoard_Active_ReturnsNavigateTo(t *testing.T) {
	c, id1, id2 := newTestServerMultiBoard(t)

	// POST /ui/boards/{id}/hide with HX-Current-URL pointing at id1 → 200 + navigate_to
	req, _ := http.NewRequest("POST", c.srv.URL+"/ui/boards/"+id1+"/hide", nil)
	req.Header.Set("X-Bankan-Token", c.token)
	req.Header.Set("HX-Current-URL", c.srv.URL+"/ui/boards/"+id1)
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]string
	decodeJSON(t, resp.Body, &body)
	assert.Equal(t, id2, body["navigate_to"])
}

func TestUI_ShowBoard_Returns204(t *testing.T) {
	c, id1, _ := newTestServerMultiBoard(t)

	// First hide it.
	req, _ := http.NewRequest("POST", c.srv.URL+"/ui/boards/"+id1+"/hide", nil)
	req.Header.Set("X-Bankan-Token", c.token)
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Then restore it.
	req, _ = http.NewRequest("POST", c.srv.URL+"/ui/boards/"+id1+"/show", nil)
	req.Header.Set("X-Bankan-Token", c.token)
	resp, _ = http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
