package server

import (
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/cmd/bankan/ui"
	"github.com/thekondor/bankan/internal/service"
)

// handleStatic serves embedded static files from the ui.StaticFS.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(ui.StaticFS, "static")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "static fs error")
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/static")
	if r2.URL.Path == "" {
		r2.URL.Path = "/"
	}
	http.FileServer(http.FS(sub)).ServeHTTP(w, r2)
}

// ─── page builders ────────────────────────────────────────────────────────────

func showArchivedFromRequest(r *http.Request) bool {
	raw := r.Header.Get("HX-Current-URL")
	if raw == "" {
		return r.URL.Query().Get("show_archived") == "true"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Query().Get("show_archived") == "true"
}

// buildTabLists returns the three board slices for the tab bar, scoped to ws.
func (s *Server) buildTabLists(ws *service.Workspace) (allTabBoards, archivedViewBoards, hiddenBoards []ui.BoardData) {
	for _, b := range ws.Reg.Boards() {
		if b.IsView {
			vb, _, err := ws.Reg.GetViewBoard(b.ID)
			if err != nil {
				continue
			}
			bd := ui.BoardData{ID: b.ID, Name: vb.Name, IsView: true, IsArchived: vb.ArchivedAt != nil, IsHidden: vb.Hidden}
			if vb.ArchivedAt != nil && !vb.Hidden {
				archivedViewBoards = append(archivedViewBoards, bd)
			} else {
				allTabBoards = append(allTabBoards, bd)
				if vb.Hidden {
					hiddenBoards = append(hiddenBoards, bd)
				}
			}
		} else {
			board, err := ws.Reg.GetBoard(b.ID)
			if err != nil {
				continue
			}
			bd := ui.BoardData{ID: b.ID, Name: board.Name, IsView: false, IsHidden: board.Hidden}
			allTabBoards = append(allTabBoards, bd)
			if board.Hidden {
				hiddenBoards = append(hiddenBoards, bd)
			}
		}
	}
	return
}

// buildAllWorkspaces converts the server's workspace list to UI types.
func (s *Server) buildAllWorkspaces(activeWsID string) []ui.WorkspaceData {
	result := make([]ui.WorkspaceData, len(s.workspaces))
	for i, ws := range s.workspaces {
		result[i] = ui.WorkspaceData{
			ID:       ws.ID,
			Name:     ws.Name,
			IsActive: ws.ID == activeWsID,
		}
	}
	return result
}

// buildBoardPage assembles the full BoardPageData for a given workspace + board ID.
func (s *Server) buildBoardPage(ws *service.Workspace, id string, showArchived bool) (ui.BoardPageData, error) {
	allTabBoards, archivedViewBoards, hiddenBoards := s.buildTabLists(ws)

	isView := ws.Reg.IsViewBoard(id)
	isReadonly := ws.Reg.IsArchivedViewBoard(id)
	var filterLabel string

	lanes, err := ws.Reg.ListLanes(id)
	if err != nil {
		return ui.BoardPageData{}, err
	}

	laneCards := make([]ui.LaneWithCards, len(lanes))
	for i, lane := range lanes {
		cards, err := ws.Reg.ListCards(id, lane.Name)
		if err != nil {
			return ui.BoardPageData{}, err
		}
		laneCards[i] = ui.LaneWithCards{Lane: lane, Cards: cards}
	}

	if showArchived && !isView {
		archivedCards, archErr := ws.Reg.ListArchivedCards(id)
		if archErr == nil && len(archivedCards) > 0 {
			laneIdx := make(map[string]int, len(laneCards))
			for i, lc := range laneCards {
				laneIdx[lc.Lane.Name] = i
			}
			var orphaned []*bankan.Card
			for _, c := range archivedCards {
				if idx, ok := laneIdx[c.ArchivedFrom]; ok {
					laneCards[idx].Cards = append(laneCards[idx].Cards, c)
				} else {
					orphaned = append(orphaned, c)
				}
			}
			if len(orphaned) > 0 {
				laneCards = append(laneCards, ui.LaneWithCards{
					Lane:      bankan.Lane{Name: "archived"},
					Cards:     orphaned,
					IsVirtual: true,
				})
			}
		}
	}

	labels, err := ws.Reg.ListLabels(id)
	if err != nil {
		labels = nil
	}

	var currentName, currentColor string
	if isView {
		vb, _, err := ws.Reg.GetViewBoard(id)
		if err == nil {
			currentName = vb.Name
			currentColor = vb.Color
			filterLabel = vb.FilterLabel
		}
	} else {
		b, err := ws.Reg.GetBoard(id)
		if err == nil {
			currentName = b.Name
			currentColor = b.Color
		}
	}

	currentWs := ui.WorkspaceData{ID: ws.ID, Name: ws.Name, IsActive: true}

	return ui.BoardPageData{
		CurrentBoard:        ui.BoardData{ID: id, Name: currentName, IsView: isView, Color: currentColor, IsArchived: isReadonly},
		CurrentWorkspace:    currentWs,
		Workspaces:          s.buildAllWorkspaces(ws.ID),
		AllBoards:           allTabBoards,
		ArchivedViewBoards:  archivedViewBoards,
		HiddenBoards:        hiddenBoards,
		Lanes:               laneCards,
		Labels:              labels,
		Token:               s.token,
		IsView:              isView,
		FilterLabel:         filterLabel,
		ShowArchived:        showArchived,
		IsReadonly:          isReadonly,
	}, nil
}

// uiBoardPath builds the UI path for a board in a workspace.
func uiBoardPath(wsID, boardID string) string {
	return "/ui/workspaces/" + wsID + "/boards/" + boardID
}

// ─── UI page handlers ─────────────────────────────────────────────────────────

// GET /
func (s *Server) handleUIRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	ws := s.firstWorkspace()
	if ws == nil {
		w.Header().Set("Content-Type", "text/html")
		_ = ui.Splash(s.token).Render(r.Context(), w)
		return
	}
	// Redirect to first workspace root (which handles board selection).
	http.Redirect(w, r, "/ui/workspaces/"+ws.ID, http.StatusFound)
}

// GET /ui/workspaces/{ws}
func (s *Server) handleUIWorkspaceRoot(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	ids := ws.Reg.BoardIDs()
	for _, id := range ids {
		if !ws.Reg.IsHiddenBoard(id) && !ws.Reg.IsArchivedViewBoard(id) {
			http.Redirect(w, r, uiBoardPath(ws.ID, id), http.StatusFound)
			return
		}
	}
	// All boards hidden/archived — show empty state.
	allTabBoards, archivedViewBoards, hiddenBoards := s.buildTabLists(ws)
	data := ui.BoardPageData{
		CurrentWorkspace:   ui.WorkspaceData{ID: ws.ID, Name: ws.Name, IsActive: true},
		Workspaces:         s.buildAllWorkspaces(ws.ID),
		AllBoards:          allTabBoards,
		ArchivedViewBoards: archivedViewBoards,
		HiddenBoards:       hiddenBoards,
		Token:              s.token,
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.NoActiveBoardsPage(data).Render(r.Context(), w)
}

// GET /ui/workspaces/{ws}/boards/{id}
func (s *Server) handleUIBoard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := r.PathValue("id")
	// Redirect stale/direct URLs that point to a hidden board back to the
	// workspace root so the user lands on NoActiveBoardsPage rather than
	// seeing a board with no visible tab.
	if ws.Reg.IsHiddenBoard(id) {
		http.Redirect(w, r, "/ui/workspaces/"+ws.ID, http.StatusFound)
		return
	}
	showArchived := r.URL.Query().Get("show_archived") == "true"
	data, err := s.buildBoardPage(ws, id, showArchived)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardPage(data).Render(r.Context(), w)
}

// GET /ui/workspaces/{ws}/boards/{id}/lanes/{lane}/cards
func (s *Server) handleUILaneCards(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	laneName := laneParam(r)
	cards, err := ws.Reg.ListCards(id, laneName)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	isReadonly := ws.Reg.IsArchivedViewBoard(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.LaneCardsFragment(cards, labels, ws.ID, id, s.token, laneName, ws.Reg.IsViewBoard(id), isReadonly).Render(r.Context(), w)
}

// GET /ui/modals/card/{id}/boards/{boardId}?ws={wsID}
// The workspace is provided via ?ws= query param; falls back to searching all workspaces.
func (s *Server) handleUICardModal(w http.ResponseWriter, r *http.Request) {
	cardID := r.PathValue("id")
	bID := r.PathValue("boardId")
	view := r.URL.Query().Get("view")

	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		// Search all workspaces for the board.
		for _, candidate := range s.workspaces {
			if candidate.Reg.HasBoard(bID) {
				ws = candidate
				break
			}
		}
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	card, err := ws.Reg.GetCard(bID, cardID)
	if err != nil {
		var notFound *service.ErrNotFound
		if errors.As(err, &notFound) && ws.Reg.IsViewBoard(bID) {
			if _, parent, vbErr := ws.Reg.GetViewBoard(bID); vbErr == nil {
				parentID := filepath.Base(parent.Dir)
				if _, cardErr := ws.Reg.GetCard(parentID, cardID); cardErr == nil {
					w.Header().Set("Content-Type", "text/html")
					_ = ui.CardNotInViewModal(cardID, ws.ID, parentID, parent.Name).Render(r.Context(), w)
					return
				}
			}
		}
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(bID)
	w.Header().Set("Content-Type", "text/html")

	switch view {
	case "move":
		lanes, _ := ws.Reg.ListLanes(bID)
		_ = ui.MoveCardModal(card, ws.ID, bID, lanes, s.token).Render(r.Context(), w)
	case "edit":
		_ = ui.EditCardModal(card, ws.ID, bID, labels, s.token).Render(r.Context(), w)
	case "unarchive":
		lanes, _ := ws.Reg.ListLanes(bID)
		_ = ui.UnarchiveCardModal(card, ws.ID, bID, lanes, s.token).Render(r.Context(), w)
	default:
		comments, _ := ws.Reg.ListComments(bID, cardID)
		for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
			comments[i], comments[j] = comments[j], comments[i]
		}
		_ = ui.CardDetailModal(ui.CardDetailData{
			Card:        card,
			WorkspaceID: ws.ID,
			BoardID:     bID,
			Labels:      labels,
			Comments:    comments,
			Token:       s.token,
			IsView:      ws.Reg.IsViewBoard(bID),
			IsReadonly:  ws.Reg.IsArchivedViewBoard(bID),
		}).Render(r.Context(), w)
	}
}

// ─── UI mutation handlers (return HTML fragments) ──────────────────────────────

// POST /ui/workspaces/{ws}/boards/{id}/cards
func (s *Server) handleUIAddCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	var req struct {
		Lane     string   `json:"lane"`
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		LabelIDs []string `json:"label_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := ws.Reg.AddCard(id, req.Lane, req.Title, req.Body, req.LabelIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CardItem(c, labels, ws.ID, id, s.token, ws.Reg.IsViewBoard(id), false).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/lanes
func (s *Server) handleUIAddLane(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := ws.Reg.AddLane(id, req.Name); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/labels
func (s *Server) handleUIAddLabel(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	name := r.FormValue("name")
	color := r.FormValue("color")
	if name == "" || color == "" {
		var req struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		_ = decodeJSON(r, &req)
		name = req.Name
		color = req.Color
	}
	if _, err := ws.Reg.AddLabel(id, name, color); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(ws.ID, id, labels, s.token).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/move
func (s *Server) handleUIMoveCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	cardID := r.PathValue("cardId")
	toLane := r.FormValue("to_lane")
	if toLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := ws.Reg.MoveCard(id, cardID, toLane); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/archive
func (s *Server) handleUIArchiveCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	cardID := r.PathValue("cardId")
	if err := ws.Reg.ArchiveCard(id, cardID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	if showArchivedFromRequest(r) {
		data, err := s.buildBoardPage(ws, id, true)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		_ = ui.BoardViewFragment(data).Render(r.Context(), w)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/restore
func (s *Server) handleUIRestoreCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	cardID := r.PathValue("cardId")
	toLane := r.FormValue("to_lane")
	if toLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := ws.Reg.RestoreCard(id, cardID, toLane); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/duplicate
func (s *Server) handleUIDuplicateCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	c, err := ws.Reg.DuplicateCard(id, r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-New-Card-ID", c.ID)
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// DELETE /ui/workspaces/{ws}/boards/{id}/lanes/{lane}
func (s *Server) handleUIRemoveLane(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	if err := ws.Reg.RemoveLane(id, laneParam(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

// DELETE /ui/workspaces/{ws}/boards/{id}/cards/{cardId}
func (s *Server) handleUIDeleteCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	if err := ws.Reg.DeleteCard(id, r.PathValue("cardId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

// POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/comment
func (s *Server) handleUIAddComment(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	cardID := r.PathValue("cardId")
	var req struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Author == "" {
		req.Author = "anonymous"
	}
	cm, err := ws.Reg.AddComment(id, cardID, req.Author, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CommentItem(*cm, false).Render(r.Context(), w)
}

// PATCH /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/comments/{commentId}
func (s *Server) handleUIUpdateComment(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	cardID := r.PathValue("cardId")
	commentID := r.PathValue("commentId")
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cm, err := ws.Reg.UpdateComment(id, cardID, commentID, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CommentItem(*cm, false).Render(r.Context(), w)
}

// PATCH /ui/workspaces/{ws}/boards/{id}/cards/{cardId}
func (s *Server) handleUIUpdateCard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	cardID := r.PathValue("cardId")
	var req struct {
		Title        *string  `json:"title"`
		Body         *string  `json:"body"`
		AddLabels    []string `json:"add_labels"`
		RemoveLabels []string `json:"remove_labels"`
		PrimaryLabel *string  `json:"primary_label"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := ws.Reg.UpdateCard(id, cardID, service.CardUpdate{
		Title:        req.Title,
		Body:         req.Body,
		AddLabels:    req.AddLabels,
		RemoveLabels: req.RemoveLabels,
		PrimaryLabel: req.PrimaryLabel,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CardItem(c, labels, ws.ID, id, s.token, ws.Reg.IsViewBoard(id), false).Render(r.Context(), w)
}

// GET /ui/modals/add-board?ws={wsID}
func (s *Server) handleUIAddBoardModal(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		ws = s.firstWorkspace()
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "no workspace available")
		return
	}
	var regularBoards []ui.BoardData
	for _, bi := range ws.Reg.Boards() {
		if bi.IsView {
			continue
		}
		b, err := ws.Reg.GetBoard(bi.ID)
		if err != nil {
			continue
		}
		regularBoards = append(regularBoards, ui.BoardData{ID: bi.ID, Name: b.Name})
	}
	currentBoardID := currentRegularBoardID(r, ws.Reg)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.AddBoardModal(ws.ID, s.token, regularBoards, currentBoardID).Render(r.Context(), w)
}

// currentRegularBoardID returns the board_id query parameter if it refers to
// a regular (non-view) board that exists in the registry.
func currentRegularBoardID(r *http.Request, reg *service.Registry) string {
	id := r.URL.Query().Get("board_id")
	if id == "" || reg.IsViewBoard(id) {
		return ""
	}
	return id
}

// GET /ui/modals/board-settings/{id}?ws={wsID}
func (s *Server) handleUIBoardSettingsModal(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		ws = s.firstWorkspace()
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "no workspace available")
		return
	}
	id := r.PathValue("id")
	var currentColor string
	if ws.Reg.IsViewBoard(id) {
		vb, _, err := ws.Reg.GetViewBoard(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		currentColor = vb.Color
	} else {
		b, err := ws.Reg.GetBoard(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		currentColor = b.Color
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardSettingsModal(ws.ID, id, currentColor, s.token).Render(r.Context(), w)
}

// PATCH /ui/workspaces/{ws}/boards/{id}/color
func (s *Server) handleUIUpdateBoardColor(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	var req struct {
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ws.Reg.UpdateBoardColor(id, req.Color); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardPage(data).Render(r.Context(), w)
}

// GET /ui/modals/manage-labels/{boardId}?ws={wsID}
func (s *Server) handleUIManageLabelsModal(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		ws = s.firstWorkspace()
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "no workspace available")
		return
	}
	id := r.PathValue("boardId")
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(ws.ID, id, labels, s.token).Render(r.Context(), w)
}

// GET /ui/workspaces/{ws}/boards/{id}/label-picker
func (s *Server) handleUILabelPicker(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.LabelPickerFragment(labels).Render(r.Context(), w)
}

// DELETE /ui/workspaces/{ws}/boards/{id}/labels/{labelId}
func (s *Server) handleUIDeleteLabel(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	labelID := r.PathValue("labelId")
	if err := ws.Reg.RemoveLabel(id, labelID, true); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(ws.ID, id, labels, s.token).Render(r.Context(), w)
}

// PATCH /ui/workspaces/{ws}/boards/{id}/labels/{labelId}
func (s *Server) handleUIRenameLabel(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	labelID := r.PathValue("labelId")
	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := ws.Reg.UpdateLabel(id, labelID, service.LabelUpdate{
		Name:  req.Name,
		Color: req.Color,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(ws.ID, id, labels, s.token).Render(r.Context(), w)
}

// GET /ui/modals/delete-label/{boardId}/{labelId}?ws={wsID}
func (s *Server) handleUIDeleteLabelDialog(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		ws = s.firstWorkspace()
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "no workspace available")
		return
	}
	bID := r.PathValue("boardId")
	labelID := r.PathValue("labelId")
	labels, err := ws.Reg.ListLabels(bID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	lbl, ok := bankan.FindLabelByID(labels, labelID)
	if !ok {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}
	isUsed, err := ws.Reg.IsLabelUsed(bID, labelID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.DeleteLabelDialog(ws.ID, bID, labelID, lbl.Name, isUsed, s.token).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/labels/{labelId}/archive
func (s *Server) handleUIArchiveLabel(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	labelID := r.PathValue("labelId")
	if err := ws.Reg.ArchiveLabel(id, labelID); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(ws.ID, id, labels, s.token).Render(r.Context(), w)
}

// GET /ui/modals/archive-view-board/{id}?ws={wsID}
func (s *Server) handleUIArchiveViewBoardModal(w http.ResponseWriter, r *http.Request) {
	ws := s.workspaceByID(r.URL.Query().Get("ws"))
	if ws == nil {
		ws = s.firstWorkspace()
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "no workspace available")
		return
	}
	id := r.PathValue("id")
	vb, parent, err := ws.Reg.GetViewBoard(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	filterLabelName := ""
	if lbl, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel); ok {
		filterLabelName = lbl.Name
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ArchiveViewBoardDialog(ws.ID, id, vb.Name, vb.FilterLabel, filterLabelName, s.token).Render(r.Context(), w)
}

// GET /ui/workspaces/{ws}/boards/{id}/labels-fragment
func (s *Server) handleUIBoardLabelsFragment(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	labels, _ := ws.Reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ViewBoardFilterLabelOptions(labels).Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/sync
func (s *Server) handleUISyncViewBoard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, ws.Reg, id) {
		return
	}
	if err := ws.Reg.SyncViewBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(ws, id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// GET /ui/markdown-hints
func (s *Server) handleUIMarkdownHints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_ = ui.MarkdownHintsModal().Render(r.Context(), w)
}

// POST /ui/workspaces/{ws}/boards/{id}/hide
func (s *Server) handleUIHideBoard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if err := ws.Reg.HideBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	// Check if the caller is currently viewing the hidden board.
	currentURL := r.Header.Get("HX-Current-URL")
	var activeBoardID string
	if u, err := url.Parse(currentURL); err == nil {
		activeBoardID = path.Base(u.Path)
	}
	if activeBoardID == id {
		// Navigate to the first non-hidden, non-archived board in this workspace.
		for _, bi := range ws.Reg.Boards() {
			if bi.ID != id && !ws.Reg.IsHiddenBoard(bi.ID) && !ws.Reg.IsArchivedViewBoard(bi.ID) {
				writeJSON(w, http.StatusOK, map[string]string{
					"navigate_to": uiBoardPath(ws.ID, bi.ID),
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"navigate_to": "/ui/workspaces/" + ws.ID})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /ui/workspaces/{ws}/boards/{id}/show
func (s *Server) handleUIShowBoard(w http.ResponseWriter, r *http.Request) {
	ws := s.requireWorkspace(w, r)
	if ws == nil {
		return
	}
	id := boardID(r)
	if err := ws.Reg.ShowBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
