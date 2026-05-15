package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bankan "github.com/thekondor/bankan"
)

// bankanBin is the path to the compiled bankan binary, built once by TestMain.
var bankanBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "bankan-testbin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: create temp dir:", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	bankanBin = filepath.Join(tmp, "bankan")
	cmd := exec.Command("go", "build", "-o", bankanBin, "github.com/thekondor/bankan/cmd/bankan")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// ─── Snapshot types ────────────────────────────────────────────────────────────
//
// Snapshots capture semantic board state without volatile fields (IDs,
// timestamps). Comparing two snapshots verifies that both code paths produced
// the same observable filesystem content.

type boardSnap struct {
	BoardName string
	Labels    []labelSnap // sorted by name
	Lanes     []laneSnap  // in lane directory order
}

type laneSnap struct {
	Name  string
	Cards []cardSnap // in card file order (insertion order)
}

type cardSnap struct {
	Title        string
	Body         string
	PrimaryLabel string   // primary label name, or "" if none
	Labels       []string // label names, sorted (excludes primary label)
	Comments     []string // comment bodies in order
}

type labelSnap struct {
	Name  string
	Color string
}

type archivedCardSnap struct {
	Title        string
	ArchivedFrom string
}

type viewBoardSnap struct {
	Name        string
	FilterLabel string     // filter label display name, not ID
	Lanes       []laneSnap // view lane order; cards resolved from parent
}

// ─── Snapshot helpers ──────────────────────────────────────────────────────────

// takeArchivedSnap reads the archived cards from a board and returns a
// slice of snapshots capturing title and archived-from lane (both volatile-free).
func takeArchivedSnap(t *testing.T, boardDir string) []archivedCardSnap {
	t.Helper()
	b, err := bankan.ReadBoard(boardDir)
	require.NoError(t, err)
	cards, err := bankan.ListArchivedCards(b)
	require.NoError(t, err)
	snaps := make([]archivedCardSnap, len(cards))
	for i, c := range cards {
		snaps[i] = archivedCardSnap{Title: c.Title, ArchivedFrom: c.ArchivedFrom}
	}
	return snaps
}

// extractIDFromOutput scans output lines for the first line starting with marker
// and returns its second whitespace-delimited token (the resource ID).
// E.g. "Card ab12c (\"Foo\") added" → "ab12c".
func extractIDFromOutput(t *testing.T, output, marker string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			fields := strings.Fields(line)
			require.True(t, len(fields) >= 2, "unexpected output line: %q", line)
			return fields[1]
		}
	}
	t.Fatalf("marker %q not found in CLI output:\n%s", marker, output)
	return ""
}

func takeBoardSnap(t *testing.T, boardDir string) boardSnap {
	t.Helper()

	b, err := bankan.ReadBoard(boardDir)
	require.NoError(t, err)

	labelsByID := make(map[string]string, len(b.Labels))
	for _, l := range b.Labels {
		labelsByID[l.ID] = l.Name
	}

	labels := make([]labelSnap, 0, len(b.Labels))
	for _, l := range b.Labels {
		labels = append(labels, labelSnap{Name: l.Name, Color: l.Color})
	}
	sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })

	lanes, err := bankan.ReadLanes(boardDir)
	require.NoError(t, err)
	laneSnaps := make([]laneSnap, 0, len(lanes))
	for _, lane := range lanes {
		cards, err := bankan.ListCards(lane)
		require.NoError(t, err)
		cs := make([]cardSnap, 0, len(cards))
		for _, c := range cards {
			comments, err := bankan.ReadComments(c.FilePath)
			require.NoError(t, err)
			bodies := make([]string, len(comments))
			for i, cm := range comments {
				bodies[i] = cm.Body
			}
			primaryLabelName := ""
			if c.PrimaryLabel != "" {
				primaryLabelName = labelsByID[c.PrimaryLabel]
			}
			cs = append(cs, cardSnap{
				Title:        c.Title,
				Body:         c.Body,
				PrimaryLabel: primaryLabelName,
				Labels:       resolveCardLabels(c.Labels, labelsByID),
				Comments:     bodies,
			})
		}
		laneSnaps = append(laneSnaps, laneSnap{Name: lane.Name, Cards: cs})
	}

	return boardSnap{BoardName: b.Name, Labels: labels, Lanes: laneSnaps}
}

func takeViewBoardSnap(t *testing.T, viewBoardDir string) viewBoardSnap {
	t.Helper()

	vb, err := bankan.ReadViewBoard(viewBoardDir)
	require.NoError(t, err)
	parent, err := bankan.ParentBoard(vb)
	require.NoError(t, err)

	labelsByID := make(map[string]string, len(parent.Labels))
	for _, l := range parent.Labels {
		labelsByID[l.ID] = l.Name
	}

	filterLabelName := ""
	if lbl, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel); ok {
		filterLabelName = lbl.Name
	}

	viewLanes, err := bankan.ReadLanes(viewBoardDir)
	require.NoError(t, err)
	laneSnaps := make([]laneSnap, 0, len(viewLanes))
	for _, lane := range viewLanes {
		cards, err := bankan.ListViewCards(vb, parent, lane)
		require.NoError(t, err)
		cs := make([]cardSnap, 0, len(cards))
		for _, c := range cards {
			cs = append(cs, cardSnap{
				Title:  c.Title,
				Body:   c.Body,
				Labels: resolveCardLabels(c.Labels, labelsByID),
			})
		}
		laneSnaps = append(laneSnaps, laneSnap{Name: lane.Name, Cards: cs})
	}

	return viewBoardSnap{
		Name:        vb.Name,
		FilterLabel: filterLabelName,
		Lanes:       laneSnaps,
	}
}

func resolveCardLabels(labelIDs []string, byID map[string]string) []string {
	names := make([]string, 0, len(labelIDs))
	for _, id := range labelIDs {
		if name, ok := byID[id]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// getFreePort returns an available TCP port on 127.0.0.1.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// ─── Board configuration ───────────────────────────────────────────────────────
//
// Sophisticated but predictably configured board used for both code paths.
//
// Board: "Sprint Board"
// Labels: Feature, Bug, Urgent, Frontend, Backend
// Lanes:  Backlog · In Progress · Review · Done
// Cards:
//   - Login Page Redesign   | Backlog      | Feature, Frontend
//   - Fix Auth Bug           | Backlog      | Bug, Backend
//   - API Rate Limiting      | In Progress  | Feature, Backend
//   - Dashboard Performance  | Review       | Feature, Frontend
//   - Database Optimization  | Done         | Bug, Backend
//
// View board: "Feature Sprint" filtered by "Feature".
//
// Operations sequence (run identically through both code paths):
//  1. AddLane "Testing"
//  2. AddCard "New Feature Implementation" in Backlog with Feature
//  3. MoveCard "Login Page Redesign" → In Progress
//  4. RenameCard "API Rate Limiting" → "API Rate Limiting v2"
//  5. AddLabel "Urgent" to "Fix Auth Bug"
//  6. RemoveLabel "Frontend" from "Login Page Redesign"
//  7. MoveCard "Dashboard Performance" → Done
//  8. ArchiveCard "Database Optimization"
//  9. RestoreCard "Database Optimization" → Done
// 10. RemoveLane "Testing"
// 11. AddComment "Needs review" to "Fix Auth Bug"
// 12. EditComment → "Needs review asap"
// 13. DuplicateCard "Fix Auth Bug" → "[dup] Fix Auth Bug"
// 14. SetPrimaryLabel "Urgent" on "Fix Auth Bug"
// 15. SyncViewBoard

// ─── Real CLI path ─────────────────────────────────────────────────────────────
//
// Each operation is performed by spawning the bankan binary as a subprocess.
// The board is initialized with the CLI and all mutations go through CLI commands.

func runViaRealCLI(t *testing.T) (boardSnap, viewBoardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "sprint-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "feature-sprint")

	labelIDs := map[string]string{} // display name → ID
	cardIDs := map[string]string{}  // current display title → card ID

	// run executes the bankan binary with the given args and returns trimmed stdout.
	// It fails the test immediately if the process exits non-zero.
	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}

	// extractID scans output lines for the first line starting with marker and
	// returns its second whitespace-delimited token (the resource ID).
	// E.g. "Label ab12c (\"Feature\", #3b82f6) added" → "ab12c".
	extractID := func(output, marker string) string {
		return extractIDFromOutput(t, output, marker)
	}

	// ── Initial board setup ────────────────────────────────────────────────

	run("board", "init", boardDir, "--name", "Sprint Board")

	addLabel := func(name, color string) {
		out := run("label", "add", "--board", boardDir, "--name", name, "--color", color)
		labelIDs[name] = extractID(out, "Label")
	}
	addLabel("Feature", "#3b82f6")
	addLabel("Bug", "#ef4444")
	addLabel("Urgent", "#f97316")
	addLabel("Frontend", "#8b5cf6")
	addLabel("Backend", "#22c55e")

	// Lane names are stored as deslugify(slugify(display)):
	//   "In Progress" → directory "02-in-progress" → Lane.Name "in progress"
	run("lane", "add", "Backlog", "--board", boardDir)
	run("lane", "add", "In Progress", "--board", boardDir)
	run("lane", "add", "Review", "--board", boardDir)
	run("lane", "add", "Done", "--board", boardDir)

	addCard := func(lane, title, body string, labelNames []string) {
		args := []string{"card", "add", "--board", boardDir,
			"--lane", lane, "--title", title, "--body", body}
		for _, n := range labelNames {
			args = append(args, "--label", labelIDs[n])
		}
		out := run(args...)
		cardIDs[title] = extractID(out, "Card")
	}
	addCard("backlog", "Login Page Redesign", "Redesign the login page", []string{"Feature", "Frontend"})
	addCard("backlog", "Fix Auth Bug", "Authentication is broken", []string{"Bug", "Backend"})
	addCard("in progress", "API Rate Limiting", "Implement rate limiting", []string{"Feature", "Backend"})
	addCard("review", "Dashboard Performance", "Optimize dashboard queries", []string{"Feature", "Frontend"})
	addCard("done", "Database Optimization", "Optimize DB indexes", []string{"Bug", "Backend"})

	run("board", "view", "create", viewDir,
		"--parent", boardDir,
		"--label", labelIDs["Feature"],
		"--name", "Feature Sprint")
	run("board", "view", "sync", "--board", viewDir)

	// ── Operations ────────────────────────────────────────────────────────

	run("lane", "add", "Testing", "--board", boardDir)
	addCard("backlog", "New Feature Implementation", "Implement the new feature", []string{"Feature"})
	run("card", "move", cardIDs["Login Page Redesign"], "--board", boardDir, "--lane", "in progress")
	run("card", "edit", cardIDs["API Rate Limiting"], "--board", boardDir, "--title", "API Rate Limiting v2")
	cardIDs["API Rate Limiting v2"] = cardIDs["API Rate Limiting"]
	delete(cardIDs, "API Rate Limiting")
	run("card", "edit", cardIDs["Fix Auth Bug"], "--board", boardDir, "--add-label", labelIDs["Urgent"])
	run("card", "edit", cardIDs["Login Page Redesign"], "--board", boardDir, "--remove-label", labelIDs["Frontend"])
	run("card", "move", cardIDs["Dashboard Performance"], "--board", boardDir, "--lane", "done")
	run("card", "archive", cardIDs["Database Optimization"], "--board", boardDir)
	run("card", "restore", cardIDs["Database Optimization"], "--board", boardDir, "--lane", "done")
	run("lane", "remove", "testing", "--board", boardDir)

	addCommentOut := run("comment", "add", cardIDs["Fix Auth Bug"],
		"--board", boardDir, "--text", "Needs review", "--author", "tester")
	commentID := extractID(addCommentOut, "Comment")
	run("comment", "edit", commentID,
		"--board", boardDir, "--card", cardIDs["Fix Auth Bug"], "--text", "Needs review asap")

	// 13. Duplicate "Fix Auth Bug"
	dupOut := run("card", "duplicate", cardIDs["Fix Auth Bug"], "--board", boardDir)
	cardIDs["[dup] Fix Auth Bug"] = extractID(dupOut, "Card")

	// 14. Set "Urgent" as primary label on "Fix Auth Bug"
	run("card", "edit", cardIDs["Fix Auth Bug"], "--board", boardDir, "--primary-label", labelIDs["Urgent"])

	run("board", "view", "sync", "--board", viewDir)

	return takeBoardSnap(t, boardDir), takeViewBoardSnap(t, viewDir)
}

// ─── Real REST path ────────────────────────────────────────────────────────────
//
// The board is created on disk as test setup, then bankan serve is spawned as a
// real process. All mutations go through HTTP requests to the live server.

func runViaRealREST(t *testing.T) (boardSnap, viewBoardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "sprint-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "feature-sprint")

	// Board must exist before the server starts so it can be registered.
	_, err := bankan.InitBoard(boardDir, "Sprint Board")
	require.NoError(t, err)

	// Start bankan serve on a free port.
	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	bid := filepath.Base(boardDir) // "sprint-board"

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	// Poll until the server responds to health-check GET.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/boards")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Confirm server is actually up before proceeding.
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	labelIDs := map[string]string{}
	cardIDs := map[string]string{}

	// restDo sends an authenticated HTTP request and returns the response.
	// The caller is responsible for closing resp.Body.
	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}

	decodeMap := func(res *http.Response) map[string]any {
		var m map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
		_ = res.Body.Close()
		return m
	}

	addLabel := func(name, color string) {
		res := restDo("POST", "/api/v1/boards/"+bid+"/labels",
			map[string]any{"name": name, "color": color})
		require.Equal(t, http.StatusCreated, res.StatusCode)
		labelIDs[name] = decodeMap(res)["id"].(string)
	}

	addLane := func(name string) {
		res := restDo("POST", "/api/v1/boards/"+bid+"/lanes",
			map[string]any{"name": name})
		require.Equal(t, http.StatusCreated, res.StatusCode)
		_ = res.Body.Close()
	}

	addCard := func(lane, title, body string, labelNames []string) {
		lids := make([]string, 0, len(labelNames))
		for _, n := range labelNames {
			lids = append(lids, labelIDs[n])
		}
		res := restDo("POST", "/api/v1/boards/"+bid+"/cards", map[string]any{
			"lane": lane, "title": title, "body": body, "label_ids": lids,
		})
		require.Equal(t, http.StatusCreated, res.StatusCode)
		cardIDs[title] = decodeMap(res)["id"].(string)
	}

	renameCard := func(oldTitle, newTitle string) {
		cid := cardIDs[oldTitle]
		res := restDo("PATCH", fmt.Sprintf("/api/v1/boards/%s/cards/%s", bid, cid),
			map[string]any{"title": newTitle})
		require.Equal(t, http.StatusOK, res.StatusCode)
		_ = res.Body.Close()
		delete(cardIDs, oldTitle)
		cardIDs[newTitle] = cid
	}

	addCardLabel := func(title, labelName string) {
		res := restDo("PATCH", fmt.Sprintf("/api/v1/boards/%s/cards/%s", bid, cardIDs[title]),
			map[string]any{"add_labels": []string{labelIDs[labelName]}})
		require.Equal(t, http.StatusOK, res.StatusCode)
		_ = res.Body.Close()
	}

	removeCardLabel := func(title, labelName string) {
		res := restDo("PATCH", fmt.Sprintf("/api/v1/boards/%s/cards/%s", bid, cardIDs[title]),
			map[string]any{"remove_labels": []string{labelIDs[labelName]}})
		require.Equal(t, http.StatusOK, res.StatusCode)
		_ = res.Body.Close()
	}

	moveCard := func(title, toLane string) {
		res := restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/move", bid, cardIDs[title]),
			map[string]any{"to_lane": toLane})
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		_ = res.Body.Close()
	}

	archiveCard := func(title string) {
		res := restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/archive", bid, cardIDs[title]), nil)
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		_ = res.Body.Close()
	}

	restoreCard := func(title, toLane string) {
		res := restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/restore", bid, cardIDs[title]),
			map[string]any{"to_lane": toLane})
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		_ = res.Body.Close()
	}

	removeLane := func(nameSlug string) {
		res := restDo("DELETE", "/api/v1/boards/"+bid+"/lanes/"+nameSlug, nil)
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		_ = res.Body.Close()
	}

	// ── Initial board setup ────────────────────────────────────────────────

	addLabel("Feature", "#3b82f6")
	addLabel("Bug", "#ef4444")
	addLabel("Urgent", "#f97316")
	addLabel("Frontend", "#8b5cf6")
	addLabel("Backend", "#22c55e")

	addLane("Backlog")
	addLane("In Progress")
	addLane("Review")
	addLane("Done")

	addCard("backlog", "Login Page Redesign", "Redesign the login page", []string{"Feature", "Frontend"})
	addCard("backlog", "Fix Auth Bug", "Authentication is broken", []string{"Bug", "Backend"})
	addCard("in progress", "API Rate Limiting", "Implement rate limiting", []string{"Feature", "Backend"})
	addCard("review", "Dashboard Performance", "Optimize dashboard queries", []string{"Feature", "Frontend"})
	addCard("done", "Database Optimization", "Optimize DB indexes", []string{"Bug", "Backend"})

	// View board + initial sync.
	res := restDo("POST", "/api/v1/view-boards", map[string]any{
		"name": "Feature Sprint", "parent_id": bid, "filter_label_id": labelIDs["Feature"],
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	viewBoardID := decodeMap(res)["id"].(string) // "feature-sprint"

	res = restDo("POST", "/api/v1/boards/"+viewBoardID+"/sync", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	// ── Operations ────────────────────────────────────────────────────────

	addLane("Testing")
	addCard("backlog", "New Feature Implementation", "Implement the new feature", []string{"Feature"})
	moveCard("Login Page Redesign", "in progress")
	renameCard("API Rate Limiting", "API Rate Limiting v2")
	addCardLabel("Fix Auth Bug", "Urgent")
	removeCardLabel("Login Page Redesign", "Frontend")
	moveCard("Dashboard Performance", "done")
	archiveCard("Database Optimization")
	restoreCard("Database Optimization", "done")
	removeLane("testing")

	res = restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments", bid, cardIDs["Fix Auth Bug"]),
		map[string]any{"author": "tester", "body": "Needs review"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	commentID := decodeMap(res)["id"].(string)

	res = restDo("PATCH", fmt.Sprintf("/api/v1/boards/%s/cards/%s/comments/%s", bid, cardIDs["Fix Auth Bug"], commentID),
		map[string]any{"body": "Needs review asap"})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	// 13. Duplicate "Fix Auth Bug"
	res = restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/duplicate", bid, cardIDs["Fix Auth Bug"]), nil)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	cardIDs["[dup] Fix Auth Bug"] = decodeMap(res)["id"].(string)

	// 14. Set "Urgent" as primary label on "Fix Auth Bug"
	res = restDo("PATCH", fmt.Sprintf("/api/v1/boards/%s/cards/%s", bid, cardIDs["Fix Auth Bug"]),
		map[string]any{"primary_label": labelIDs["Urgent"]})
	require.Equal(t, http.StatusOK, res.StatusCode)
	_ = res.Body.Close()

	res = restDo("POST", "/api/v1/boards/"+viewBoardID+"/sync", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	return takeBoardSnap(t, boardDir), takeViewBoardSnap(t, viewDir)
}

// ─── Main test ─────────────────────────────────────────────────────────────────

// TestLifecycle_CLIvsREST_SophisticatedBoardEquivalence builds the bankan
// binary, then runs an identical sequence of board operations through two
// independent paths — real CLI subprocesses and a real bankan serve process —
// and asserts that both paths leave the filesystem in semantically identical
// state.
func TestLifecycle_CLIvsREST_SophisticatedBoardEquivalence(t *testing.T) {
	cliBoard, cliView := runViaRealCLI(t)
	restBoard, restView := runViaRealREST(t)

	assert.Equal(t, cliBoard, restBoard,
		"board filesystem state must be identical between real CLI and real REST paths")
	assert.Equal(t, cliView, restView,
		"view board filesystem state must be identical between real CLI and real REST paths")
}

// ─── Archive / Restore equivalence test ───────────────────────────────────────
//
// A focused test verifying that archive and restore operations produce identical
// filesystem state through both the real CLI and the real REST server.
//
// Scenario:
//   1. Init board with one lane "todo"
//   2. Add two cards: "Fix the Bug" and "Ship the Feature"
//   3. Archive "Fix the Bug"
//   4. Snapshot archived cards (must include only "Fix the Bug", from "todo")
//   5. Restore "Fix the Bug" back to "todo"
//   6. Snapshot full board (both cards in "todo", nothing in archive)

func runArchiveRestoreViaRealCLI(t *testing.T) ([]archivedCardSnap, boardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "archive-test")
	require.NoError(t, os.Mkdir(boardDir, 0o755))

	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}

	run("board", "init", boardDir, "--name", "Archive Test Board")
	run("lane", "add", "todo", "--board", boardDir)

	out := run("card", "add", "--board", boardDir, "--lane", "todo", "--title", "Fix the Bug", "--body", "critical")
	bugCardID := extractIDFromOutput(t, out, "Card")

	run("card", "add", "--board", boardDir, "--lane", "todo", "--title", "Ship the Feature", "--body", "")

	// Archive "Fix the Bug"
	run("card", "archive", bugCardID, "--board", boardDir)
	archivedSnap := takeArchivedSnap(t, boardDir)

	// Restore back to todo
	run("card", "restore", bugCardID, "--board", boardDir, "--lane", "todo")

	return archivedSnap, takeBoardSnap(t, boardDir)
}

func runArchiveRestoreViaRealREST(t *testing.T) ([]archivedCardSnap, boardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "archive-test")
	require.NoError(t, os.Mkdir(boardDir, 0o755))

	_, err := bankan.InitBoard(boardDir, "Archive Test Board")
	require.NoError(t, err)

	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	bid := filepath.Base(boardDir) // "archive-test"

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/boards")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}

	decodeMap := func(res *http.Response) map[string]any {
		var m map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
		_ = res.Body.Close()
		return m
	}

	// Add lane and cards
	res := restDo("POST", "/api/v1/boards/"+bid+"/lanes", map[string]any{"name": "todo"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	_ = res.Body.Close()

	res = restDo("POST", "/api/v1/boards/"+bid+"/cards", map[string]any{
		"lane": "todo", "title": "Fix the Bug", "body": "critical",
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	bugCardID := decodeMap(res)["id"].(string)

	res = restDo("POST", "/api/v1/boards/"+bid+"/cards", map[string]any{
		"lane": "todo", "title": "Ship the Feature", "body": "",
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	_ = res.Body.Close()

	// Archive "Fix the Bug"
	res = restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/archive", bid, bugCardID), nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	archivedSnap := takeArchivedSnap(t, boardDir)

	// Restore back to todo
	res = restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/restore", bid, bugCardID),
		map[string]any{"to_lane": "todo"})
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	return archivedSnap, takeBoardSnap(t, boardDir)
}

// TestLifecycle_CLIvsREST_ArchiveRestore verifies that archive and restore
// operations leave the filesystem in semantically identical state whether
// executed through the real CLI or the real REST server.
func TestLifecycle_CLIvsREST_ArchiveRestore(t *testing.T) {
	cliArchived, cliBoard := runArchiveRestoreViaRealCLI(t)
	restArchived, restBoard := runArchiveRestoreViaRealREST(t)

	assert.Equal(t, cliArchived, restArchived,
		"archived card state must be identical between real CLI and real REST paths")
	assert.Equal(t, cliBoard, restBoard,
		"board state after restore must be identical between real CLI and real REST paths")
}

// ─── ViewBoard archive equivalence test ───────────────────────────────────────
//
// Verifies that archiving a view board (with --archive-label / archive_label:true)
// produces identical filesystem state through the CLI and the REST server.
//
// Scenario:
//  1. Init parent board with label "Sprint" and lane "backlog"
//  2. Add card "Sprint Card" labelled Sprint
//  3. Create view board "Sprint View" filtered by Sprint; sync
//  4. Archive the view board with archive-label=true
//  5. Assert: view board archived_at is set, filter label renamed to "💼 Sprint"

type viewBoardArchiveSnap struct {
	IsArchived      bool
	FilterLabelName string // label name on the parent board
}

func takeViewBoardArchiveSnap(t *testing.T, viewBoardDir string) viewBoardArchiveSnap {
	t.Helper()
	vb, err := bankan.ReadViewBoard(viewBoardDir)
	require.NoError(t, err)
	parent, err := bankan.ParentBoard(vb)
	require.NoError(t, err)
	labelName := ""
	if lbl, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel); ok {
		labelName = lbl.Name
	}
	return viewBoardArchiveSnap{
		IsArchived:      vb.ArchivedAt != nil,
		FilterLabelName: labelName,
	}
}

func runViewBoardArchiveViaRealCLI(t *testing.T) viewBoardArchiveSnap {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "parent-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "sprint-view")

	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}

	run("board", "init", boardDir, "--name", "Parent Board")
	run("lane", "add", "backlog", "--board", boardDir)

	labelOut := run("label", "add", "--board", boardDir, "--name", "Sprint", "--color", "#3b82f6")
	labelID := extractIDFromOutput(t, labelOut, "Label")

	cardOut := run("card", "add", "--board", boardDir, "--lane", "backlog",
		"--title", "Sprint Card", "--body", "body", "--label", labelID)
	_ = extractIDFromOutput(t, cardOut, "Card")

	run("board", "view", "create", viewDir,
		"--parent", boardDir, "--label", labelID, "--name", "Sprint View")
	run("board", "view", "sync", "--board", viewDir)

	run("board", "view", "archive", "--board", viewDir, "--archive-label")

	return takeViewBoardArchiveSnap(t, viewDir)
}

func runViewBoardArchiveViaRealREST(t *testing.T) viewBoardArchiveSnap {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "parent-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "sprint-view")

	_, err := bankan.InitBoard(boardDir, "Parent Board")
	require.NoError(t, err)

	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	bid := filepath.Base(boardDir) // "parent-board"

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, hErr := http.Get(baseURL + "/api/v1/boards")
		if hErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}

	decodeMap := func(res *http.Response) map[string]any {
		var m map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
		_ = res.Body.Close()
		return m
	}

	// Add lane + label + card
	res := restDo("POST", "/api/v1/boards/"+bid+"/lanes", map[string]any{"name": "backlog"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	_ = res.Body.Close()

	res = restDo("POST", "/api/v1/boards/"+bid+"/labels",
		map[string]any{"name": "Sprint", "color": "#3b82f6"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	labelID := decodeMap(res)["id"].(string)

	res = restDo("POST", "/api/v1/boards/"+bid+"/cards", map[string]any{
		"lane": "backlog", "title": "Sprint Card", "body": "body",
		"label_ids": []string{labelID},
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	_ = res.Body.Close()

	// Create + sync view board
	res = restDo("POST", "/api/v1/view-boards", map[string]any{
		"name": "Sprint View", "parent_id": bid, "filter_label_id": labelID,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	viewID := decodeMap(res)["id"].(string)

	res = restDo("POST", "/api/v1/boards/"+viewID+"/sync", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	// Archive view board with archive_label=true
	res = restDo("POST", "/api/v1/boards/"+viewID+"/archive",
		map[string]any{"archive_label": true})
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	// Verify mutation is now rejected (403)
	res = restDo("POST", "/api/v1/boards/"+viewID+"/cards", map[string]any{
		"lane": "backlog", "title": "Should Fail",
	})
	assert.Equal(t, http.StatusForbidden, res.StatusCode, "archived view board must reject card creation")
	_ = res.Body.Close()

	_ = viewDir // suppress unused warning; snap reads from disk
	return takeViewBoardArchiveSnap(t, viewDir)
}

// TestLifecycle_CLIvsREST_ViewBoardArchive verifies that archiving a view board
// (with archive-label) produces identical filesystem state through the CLI and
// the REST server, and that the REST server correctly blocks mutations on the
// archived board.
func TestLifecycle_CLIvsREST_ViewBoardArchive(t *testing.T) {
	cliSnap := runViewBoardArchiveViaRealCLI(t)
	restSnap := runViewBoardArchiveViaRealREST(t)

	assert.Equal(t, cliSnap, restSnap,
		"view board archive state must be identical between real CLI and real REST paths")
	assert.True(t, cliSnap.IsArchived, "view board must be marked archived")
	assert.Equal(t, "💼 Sprint", cliSnap.FilterLabelName,
		"filter label must be prefixed with 💼 when archive-label is used")
}
// ─── Card reorder equivalence test ───────────────────────────────────────────
//
// Verifies that reordering a card within a view board lane produces identical
// filesystem state (both view stubs and parent labeled cards) through the real
// CLI and the real REST server.
//
// Scenario:
//  1. Init parent board with label "Sprint" and lane "Backlog"
//  2. Add three Sprint-labeled cards: "Alpha", "Beta", "Gamma"
//  3. Add one unlabeled card "Unlabeled" between Alpha and Beta
//  4. Create view board "Sprint View" filtered by Sprint; sync
//  5. Reorder "Alpha" (view index 0) to view index 2 (last Sprint position)
//  6. Assert: view stub order = Beta, Gamma, Alpha
//             parent labeled order = Beta, Gamma, Alpha (Unlabeled unaffected)

func runCardReorderViaRealCLI(t *testing.T) (boardSnap, viewBoardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "reorder-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "sprint-view")

	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}
	extractID := func(output, marker string) string {
		return extractIDFromOutput(t, output, marker)
	}

	run("board", "init", boardDir, "--name", "Reorder Board")
	run("lane", "add", "Backlog", "--board", boardDir)

	labelOut := run("label", "add", "--board", boardDir, "--name", "Sprint", "--color", "#3b82f6")
	labelID := extractID(labelOut, "Label")

	addCard := func(title string, labels []string) string {
		args := []string{"card", "add", "--board", boardDir, "--lane", "backlog", "--title", title, "--body", ""}
		for _, l := range labels {
			args = append(args, "--label", l)
		}
		return extractID(run(args...), "Card")
	}

	alphaID := addCard("Alpha", []string{labelID})
	addCard("Unlabeled", nil)
	betaID := addCard("Beta", []string{labelID})
	addCard("Gamma", []string{labelID})

	run("board", "view", "create", viewDir, "--parent", boardDir, "--label", labelID, "--name", "Sprint View")
	run("board", "view", "sync", "--board", viewDir)

	// Reorder Alpha (view index 0) to view index 2 using the view board.
	run("card", "reorder", alphaID, "2", "--board", viewDir)
	_ = betaID

	return takeBoardSnap(t, boardDir), takeViewBoardSnap(t, viewDir)
}

func runCardReorderViaRealREST(t *testing.T) (boardSnap, viewBoardSnap) {
	t.Helper()

	rootDir := t.TempDir()
	boardDir := filepath.Join(rootDir, "reorder-board")
	require.NoError(t, os.Mkdir(boardDir, 0o755))
	viewDir := filepath.Join(rootDir, "sprint-view")

	_, err := bankan.InitBoard(boardDir, "Reorder Board")
	require.NoError(t, err)

	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	bid := filepath.Base(boardDir) // "reorder-board"

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, hErr := http.Get(baseURL + "/api/v1/boards")
		if hErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}
	decodeMap := func(res *http.Response) map[string]any {
		var m map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&m))
		_ = res.Body.Close()
		return m
	}

	// Setup: lane, label, cards.
	res := restDo("POST", "/api/v1/boards/"+bid+"/lanes", map[string]any{"name": "Backlog"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	_ = res.Body.Close()

	res = restDo("POST", "/api/v1/boards/"+bid+"/labels",
		map[string]any{"name": "Sprint", "color": "#3b82f6"})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	labelID := decodeMap(res)["id"].(string)

	addCard := func(title string, labels []string) string {
		res := restDo("POST", "/api/v1/boards/"+bid+"/cards", map[string]any{
			"lane": "backlog", "title": title, "body": "", "label_ids": labels,
		})
		require.Equal(t, http.StatusCreated, res.StatusCode)
		return decodeMap(res)["id"].(string)
	}

	alphaID := addCard("Alpha", []string{labelID})
	addCard("Unlabeled", []string{})
	addCard("Beta", []string{labelID})
	addCard("Gamma", []string{labelID})

	// Create + sync view board.
	res = restDo("POST", "/api/v1/view-boards", map[string]any{
		"name": "Sprint View", "parent_id": bid, "filter_label_id": labelID,
	})
	require.Equal(t, http.StatusCreated, res.StatusCode)
	viewBoardID := decodeMap(res)["id"].(string)

	res = restDo("POST", "/api/v1/boards/"+viewBoardID+"/sync", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	// Reorder Alpha (view index 0) to view index 2 via the view board endpoint.
	res = restDo("POST", fmt.Sprintf("/api/v1/boards/%s/cards/%s/reorder", viewBoardID, alphaID),
		map[string]any{"new_index": 2})
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	_ = viewDir
	return takeBoardSnap(t, boardDir), takeViewBoardSnap(t, viewDir)
}

// TestLifecycle_CLIvsREST_CardReorder verifies that reordering a card in a
// view board produces identical filesystem state (view stubs + parent labeled
// card order) through the real CLI and the real REST server.
func TestLifecycle_CLIvsREST_CardReorder(t *testing.T) {
	cliBoard, cliView := runCardReorderViaRealCLI(t)
	restBoard, restView := runCardReorderViaRealREST(t)

	assert.Equal(t, cliBoard, restBoard,
		"parent board filesystem state must be identical between real CLI and real REST paths")
	assert.Equal(t, cliView, restView,
		"view board filesystem state must be identical between real CLI and real REST paths")

	// The view must show Beta, Gamma, Alpha in that order.
	require.Len(t, cliView.Lanes, 1)
	require.Len(t, cliView.Lanes[0].Cards, 3)
	assert.Equal(t, "Beta", cliView.Lanes[0].Cards[0].Title)
	assert.Equal(t, "Gamma", cliView.Lanes[0].Cards[1].Title)
	assert.Equal(t, "Alpha", cliView.Lanes[0].Cards[2].Title)
}
// ─── Board reorder equivalence test ──────────────────────────────────────────
//
// Verifies that reordering boards produces identical persisted state through
// the real CLI (bankan board reorder) and the real REST server
// (POST /api/v1/boards/reorder).
//
// Scenario:
//  1. Create root dir with two boards: "board-alpha" and "board-beta"
//     (alphabetical creation order gives default sort: alpha → beta)
//  2. Reorder to: beta first, alpha second
//  3. Assert: on-disk order field reflects the new order (beta=1, alpha=2)
//  4. Assert: a freshly loaded registry returns boards in the new order

type boardOrderSnap struct {
	IDs []string // board IDs in display order (as read back from disk)
}

func runBoardReorderViaRealCLI(t *testing.T) boardOrderSnap {
	t.Helper()

	rootDir := t.TempDir()
	alphaDir := filepath.Join(rootDir, "board-alpha")
	betaDir := filepath.Join(rootDir, "board-beta")
	require.NoError(t, os.Mkdir(alphaDir, 0o755))
	require.NoError(t, os.Mkdir(betaDir, 0o755))

	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}

	run("board", "init", alphaDir, "--name", "Alpha")
	run("board", "init", betaDir, "--name", "Beta")

	// Reorder: beta first, alpha second.
	run("board", "reorder", "--root", rootDir, "board-beta", "board-alpha")

	// Reload from disk to verify persistence.
	bAlpha, err := bankan.ReadBoard(alphaDir)
	require.NoError(t, err)
	bBeta, err := bankan.ReadBoard(betaDir)
	require.NoError(t, err)

	type idOrder struct {
		id    string
		order int
	}
	items := []idOrder{
		{"board-alpha", bAlpha.Order},
		{"board-beta", bBeta.Order},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].order < items[j].order })
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.id
	}
	return boardOrderSnap{IDs: ids}
}

func runBoardReorderViaRealREST(t *testing.T) boardOrderSnap {
	t.Helper()

	rootDir := t.TempDir()
	alphaDir := filepath.Join(rootDir, "board-alpha")
	betaDir := filepath.Join(rootDir, "board-beta")
	require.NoError(t, os.Mkdir(alphaDir, 0o755))
	require.NoError(t, os.Mkdir(betaDir, 0o755))
	_, err := bankan.InitBoard(alphaDir, "Alpha")
	require.NoError(t, err)
	_, err = bankan.InitBoard(betaDir, "Beta")
	require.NoError(t, err)

	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, hErr := http.Get(baseURL + "/api/v1/boards")
		if hErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}

	// Reorder: beta first, alpha second.
	res := restDo("POST", "/api/v1/boards/reorder",
		map[string]any{"ids": []string{"board-beta", "board-alpha"}})
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	// Verify via the list endpoint (already ordered by display order).
	res = restDo("GET", "/api/v1/boards", nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var boards []map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&boards))
	_ = res.Body.Close()

	ids := make([]string, len(boards))
	for i, b := range boards {
		ids[i] = b["id"].(string)
	}
	return boardOrderSnap{IDs: ids}
}

// TestLifecycle_CLIvsREST_BoardReorder verifies that reordering boards via the
// CLI and the REST server produce identical persisted display order.
func TestLifecycle_CLIvsREST_BoardReorder(t *testing.T) {
	cliSnap := runBoardReorderViaRealCLI(t)
	restSnap := runBoardReorderViaRealREST(t)

	assert.Equal(t, cliSnap, restSnap,
		"board display order must be identical between real CLI and real REST paths")
	assert.Equal(t, []string{"board-beta", "board-alpha"}, cliSnap.IDs,
		"board-beta must sort first after reorder")
}

// ─── Hide / Unhide equivalence test ──────────────────────────────────────────
//
// Verifies that hiding and unhiding a board produces identical persisted state
// through the real CLI (bankan board hide / board unhide) and the real REST
// server (POST /api/v1/boards/{id}/hide and /api/v1/boards/{id}/show).
//
// Scenario:
//  1. Create two boards: "board-alpha" and "board-beta"
//  2. Hide "board-alpha"
//  3. Assert board-alpha has hidden=true, board-beta has hidden=false
//  4. Unhide "board-alpha"
//  5. Assert both boards have hidden=false

type boardHiddenSnap struct {
	AlphaHidden bool
	BetaHidden  bool
}

func runHideUnhideViaRealCLI(t *testing.T) (afterHide, afterUnhide boardHiddenSnap) {
	t.Helper()

	rootDir := t.TempDir()
	alphaDir := filepath.Join(rootDir, "board-alpha")
	betaDir := filepath.Join(rootDir, "board-beta")
	require.NoError(t, os.Mkdir(alphaDir, 0o755))
	require.NoError(t, os.Mkdir(betaDir, 0o755))

	run := func(args ...string) string {
		cmd := exec.Command(bankanBin, args...)
		out, err := cmd.CombinedOutput()
		outStr := strings.TrimSpace(string(out))
		require.NoError(t, err, "bankan %s failed\n%s", strings.Join(args, " "), outStr)
		return outStr
	}

	run("board", "init", alphaDir, "--name", "Alpha")
	run("board", "init", betaDir, "--name", "Beta")

	run("board", "hide", "board-alpha", "--root", rootDir)

	bAlpha, err := bankan.ReadBoard(alphaDir)
	require.NoError(t, err)
	bBeta, err := bankan.ReadBoard(betaDir)
	require.NoError(t, err)
	afterHide = boardHiddenSnap{AlphaHidden: bAlpha.Hidden, BetaHidden: bBeta.Hidden}

	run("board", "unhide", "board-alpha", "--root", rootDir)

	bAlpha, err = bankan.ReadBoard(alphaDir)
	require.NoError(t, err)
	bBeta, err = bankan.ReadBoard(betaDir)
	require.NoError(t, err)
	afterUnhide = boardHiddenSnap{AlphaHidden: bAlpha.Hidden, BetaHidden: bBeta.Hidden}

	return afterHide, afterUnhide
}

func runHideUnhideViaRealREST(t *testing.T) (afterHide, afterUnhide boardHiddenSnap) {
	t.Helper()

	rootDir := t.TempDir()
	alphaDir := filepath.Join(rootDir, "board-alpha")
	betaDir := filepath.Join(rootDir, "board-beta")
	require.NoError(t, os.Mkdir(alphaDir, 0o755))
	require.NoError(t, os.Mkdir(betaDir, 0o755))
	_, err := bankan.InitBoard(alphaDir, "Alpha")
	require.NoError(t, err)
	_, err = bankan.InitBoard(betaDir, "Beta")
	require.NoError(t, err)

	port := getFreePort(t)
	const token = "test-token"
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	serverCmd := exec.Command(bankanBin, "serve", rootDir,
		"--port", strconv.Itoa(port),
		"--token", token,
		"--bind", "127.0.0.1")
	serverCmd.Stdout = io.Discard
	serverCmd.Stderr = io.Discard
	require.NoError(t, serverCmd.Start())
	t.Cleanup(func() {
		if serverCmd.Process != nil {
			_ = serverCmd.Process.Kill()
			serverCmd.Wait() //nolint:errcheck
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, hErr := http.Get(baseURL + "/api/v1/boards")
		if hErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get(baseURL + "/api/v1/boards")
	require.NoError(t, err, "server did not start within 5 seconds")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	restDo := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, reqErr := http.NewRequest(method, baseURL+path, r)
		require.NoError(t, reqErr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Bankan-Token", token)
		res, doErr := http.DefaultClient.Do(req)
		require.NoError(t, doErr)
		return res
	}

	res := restDo("POST", "/api/v1/boards/board-alpha/hide", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	bAlpha, err := bankan.ReadBoard(alphaDir)
	require.NoError(t, err)
	bBeta, err := bankan.ReadBoard(betaDir)
	require.NoError(t, err)
	afterHide = boardHiddenSnap{AlphaHidden: bAlpha.Hidden, BetaHidden: bBeta.Hidden}

	res = restDo("POST", "/api/v1/boards/board-alpha/show", nil)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	_ = res.Body.Close()

	bAlpha, err = bankan.ReadBoard(alphaDir)
	require.NoError(t, err)
	bBeta, err = bankan.ReadBoard(betaDir)
	require.NoError(t, err)
	afterUnhide = boardHiddenSnap{AlphaHidden: bAlpha.Hidden, BetaHidden: bBeta.Hidden}

	return afterHide, afterUnhide
}

// TestLifecycle_CLIvsREST_HideUnhideBoard verifies that hiding and unhiding a
// board produces identical persisted filesystem state through the real CLI and
// the real REST server.
func TestLifecycle_CLIvsREST_HideUnhideBoard(t *testing.T) {
	cliAfterHide, cliAfterUnhide := runHideUnhideViaRealCLI(t)
	restAfterHide, restAfterUnhide := runHideUnhideViaRealREST(t)

	assert.Equal(t, cliAfterHide, restAfterHide,
		"hidden state after hide must be identical between real CLI and real REST paths")
	assert.Equal(t, cliAfterUnhide, restAfterUnhide,
		"hidden state after unhide must be identical between real CLI and real REST paths")

	assert.True(t, cliAfterHide.AlphaHidden, "board-alpha must be hidden after hide")
	assert.False(t, cliAfterHide.BetaHidden, "board-beta must not be hidden")
	assert.False(t, cliAfterUnhide.AlphaHidden, "board-alpha must be visible after unhide")
	assert.False(t, cliAfterUnhide.BetaHidden, "board-beta must remain visible")
}
