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

// showArchivedFromRequest returns true when the browser page that triggered
// the current HTMX request had ?show_archived=true in its URL. HTMX sends the
// originating page URL in the HX-Current-URL header; for direct (non-HTMX)
// requests the query param on r.URL is used as a fallback.
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

// buildBoardPage assembles the full BoardPageData for a given board ID.
// When showArchived is true, archived cards are loaded and merged back into
// their original lanes (read-only). Cards whose original lane no longer exists
// are collected into a virtual "archived" lane appended at the end.
// buildTabLists returns the three board slices used to render the tab bar.
// allTabBoards: all non-archived boards (visible + hidden) in registry order so
//
//	hidden tabs are in the correct DOM position for client-side restore.
//
// archivedViewBoards: archived view boards shown in the overflow dropdown only.
// hiddenBoards: filtered copy of allTabBoards where IsHidden is true, used only
//
//	for the overflow dropdown.
func (s *Server) buildTabLists() (allTabBoards, archivedViewBoards, hiddenBoards []ui.BoardData) {
	for _, b := range s.reg.Boards() {
		if b.IsView {
			vb, _, err := s.reg.GetViewBoard(b.ID)
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
			board, err := s.reg.GetBoard(b.ID)
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

func (s *Server) buildBoardPage(id string, showArchived bool) (ui.BoardPageData, error) {
	allTabBoards, archivedViewBoards, hiddenBoards := s.buildTabLists()

	isView := s.reg.IsViewBoard(id)
	isReadonly := s.reg.IsArchivedViewBoard(id)
	var filterLabel string

	lanes, err := s.reg.ListLanes(id)
	if err != nil {
		return ui.BoardPageData{}, err
	}

	laneCards := make([]ui.LaneWithCards, len(lanes))
	for i, lane := range lanes {
		cards, err := s.reg.ListCards(id, lane.Name)
		if err != nil {
			return ui.BoardPageData{}, err
		}
		laneCards[i] = ui.LaneWithCards{Lane: lane, Cards: cards}
	}

	// When show_archived is requested, load archived cards and merge them into
	// the board. Each archived card is appended to its original lane's card
	// list (read-only). Cards whose lane was subsequently deleted are collected
	// into a virtual lane rendered at the end.
	if showArchived && !isView {
		archivedCards, archErr := s.reg.ListArchivedCards(id)
		if archErr == nil && len(archivedCards) > 0 {
			// Build a lookup: lane name → index in laneCards.
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
			// Orphaned archived cards (original lane deleted) go into a virtual lane.
			if len(orphaned) > 0 {
				laneCards = append(laneCards, ui.LaneWithCards{
					Lane:      bankan.Lane{Name: "archived"},
					Cards:     orphaned,
					IsVirtual: true,
				})
			}
		}
	}

	labels, err := s.reg.ListLabels(id)
	if err != nil {
		labels = nil
	}

	var currentName, currentColor string
	if isView {
		vb, _, err := s.reg.GetViewBoard(id)
		if err == nil {
			currentName = vb.Name
			currentColor = vb.Color
			filterLabel = vb.FilterLabel
		}
	} else {
		b, err := s.reg.GetBoard(id)
		if err == nil {
			currentName = b.Name
			currentColor = b.Color
		}
	}

	return ui.BoardPageData{
		CurrentBoard:       ui.BoardData{ID: id, Name: currentName, IsView: isView, Color: currentColor, IsArchived: isReadonly},
		AllBoards:          allTabBoards,
		ArchivedViewBoards: archivedViewBoards,
		HiddenBoards:       hiddenBoards,
		Lanes:              laneCards,
		Labels:             labels,
		Token:              s.token,
		IsView:             isView,
		FilterLabel:        filterLabel,
		ShowArchived:       showArchived,
		IsReadonly:         isReadonly,
	}, nil
}

// ─── UI page handlers ─────────────────────────────────────────────────────────

// GET /
func (s *Server) handleUIRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	ids := s.reg.BoardIDs()
	if len(ids) == 0 {
		w.Header().Set("Content-Type", "text/html")
		_ = ui.Splash(s.token).Render(r.Context(), w)
		return
	}
	// Redirect to the first board that is visible (not hidden, not an archived view board).
	for _, id := range ids {
		if !s.reg.IsHiddenBoard(id) && !s.reg.IsArchivedViewBoard(id) {
			http.Redirect(w, r, "/ui/boards/"+id, http.StatusFound)
			return
		}
	}
	// All boards are hidden or archived — show empty state with the tab header so
	// the user can still access the ▽ dropdown to restore a hidden board.
	allTabBoards, archivedViewBoards, hiddenBoards := s.buildTabLists()
	data := ui.BoardPageData{
		AllBoards:          allTabBoards,
		ArchivedViewBoards: archivedViewBoards,
		HiddenBoards:       hiddenBoards,
		Token:              s.token,
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.NoActiveBoardsPage(data).Render(r.Context(), w)
}

// GET /ui/boards/{id}
func (s *Server) handleUIBoard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	showArchived := r.URL.Query().Get("show_archived") == "true"
	data, err := s.buildBoardPage(id, showArchived)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardPage(data).Render(r.Context(), w)
}

// GET /ui/boards/{id}/lanes/{lane}/cards
func (s *Server) handleUILaneCards(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	laneName := laneParam(r)
	cards, err := s.reg.ListCards(id, laneName)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(id)
	isReadonly := s.reg.IsArchivedViewBoard(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.LaneCardsFragment(cards, labels, id, s.token, laneName, s.reg.IsViewBoard(id), isReadonly).Render(r.Context(), w)
}

// GET /ui/modals/card/{id}/boards/{boardId}
func (s *Server) handleUICardModal(w http.ResponseWriter, r *http.Request) {
	cardID  := r.PathValue("id")
	boardID := r.PathValue("boardId")
	view    := r.URL.Query().Get("view")

	card, err := s.reg.GetCard(boardID, cardID)
	if err != nil {
		// When a view board permalink is opened but the card stub was removed from
		// the view, check whether the card still lives on the parent board. If so,
		// render a helpful modal instead of a plain 404, with a link to the card
		// on the parent board.
		var notFound *service.ErrNotFound
		if errors.As(err, &notFound) && s.reg.IsViewBoard(boardID) {
			if _, parent, vbErr := s.reg.GetViewBoard(boardID); vbErr == nil {
				parentID := filepath.Base(parent.Dir)
				if _, cardErr := s.reg.GetCard(parentID, cardID); cardErr == nil {
					w.Header().Set("Content-Type", "text/html")
					_ = ui.CardNotInViewModal(cardID, parentID, parent.Name).Render(r.Context(), w)
					return
				}
			}
		}
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(boardID)
	w.Header().Set("Content-Type", "text/html")

	switch view {
	case "move":
		lanes, _ := s.reg.ListLanes(boardID)
		_ = ui.MoveCardModal(card, boardID, lanes, s.token).Render(r.Context(), w)
	case "edit":
		_ = ui.EditCardModal(card, boardID, labels, s.token).Render(r.Context(), w)
	case "unarchive":
		lanes, _ := s.reg.ListLanes(boardID)
		_ = ui.UnarchiveCardModal(card, boardID, lanes, s.token).Render(r.Context(), w)
	default:
		comments, _ := s.reg.ListComments(boardID, cardID)
		for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
			comments[i], comments[j] = comments[j], comments[i]
		}
		_ = ui.CardDetailModal(ui.CardDetailData{
			Card:       card,
			BoardID:    boardID,
			Labels:     labels,
			Comments:   comments,
			Token:      s.token,
			IsView:     s.reg.IsViewBoard(boardID),
			IsReadonly: s.reg.IsArchivedViewBoard(boardID),
		}).Render(r.Context(), w)
	}
}

// ─── UI mutation handlers (return HTML fragments) ──────────────────────────────

// POST /ui/boards/{id}/cards  → returns card HTML fragment
func (s *Server) handleUIAddCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
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
	c, err := s.reg.AddCard(id, req.Lane, req.Title, req.Body, req.LabelIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CardItem(c, labels, id, s.token, s.reg.IsViewBoard(id), false).Render(r.Context(), w)
}

// POST /ui/boards/{id}/lanes  → returns full board view HTML
func (s *Server) handleUIAddLane(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.reg.AddLane(id, req.Name); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/boards/{id}/labels → returns manage-labels modal HTML
func (s *Server) handleUIAddLabel(w http.ResponseWriter, r *http.Request) {
	id    := boardID(r)
	name  := r.FormValue("name")
	color := r.FormValue("color")
	if name == "" || color == "" {
		// Try JSON
		var req struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		}
		_ = decodeJSON(r, &req)
		name  = req.Name
		color = req.Color
	}
	if _, err := s.reg.AddLabel(id, name, color); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(id, labels, s.token).Render(r.Context(), w)
}

// POST /ui/boards/{id}/cards/{cardId}/move → returns full board view HTML
func (s *Server) handleUIMoveCard(w http.ResponseWriter, r *http.Request) {
	id     := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	cardID := r.PathValue("cardId")
	// HTMX posts application/x-www-form-urlencoded; hx-vals merges into form params.
	toLane := r.FormValue("to_lane")
	if toLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := s.reg.MoveCard(id, cardID, toLane); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/boards/{id}/cards/{cardId}/archive
// When show_archived is active, returns the full board view so the card
// re-appears in its lane as archived (read-only). Otherwise returns an empty
// 200 so HTMX / JS can simply remove the card element.
func (s *Server) handleUIArchiveCard(w http.ResponseWriter, r *http.Request) {
	id     := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	cardID := r.PathValue("cardId")
	if err := s.reg.ArchiveCard(id, cardID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	if showArchivedFromRequest(r) {
		data, err := s.buildBoardPage(id, true)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		_ = ui.BoardViewFragment(data).Render(r.Context(), w)
		return
	}
	// Return empty so JS removes the card element directly.
	w.WriteHeader(http.StatusOK)
}

// POST /ui/boards/{id}/cards/{cardId}/restore → restores an archived card and
// returns the full board view. The show_archived state is preserved from the
// originating page URL (HX-Current-URL header).
func (s *Server) handleUIRestoreCard(w http.ResponseWriter, r *http.Request) {
	id     := boardID(r)
	cardID := r.PathValue("cardId")
	// HTMX posts application/x-www-form-urlencoded; hx-vals merges into form params.
	toLane := r.FormValue("to_lane")
	if toLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := s.reg.RestoreCard(id, cardID, toLane); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// POST /ui/boards/{id}/cards/{cardId}/duplicate → duplicates card and returns
// the updated board view fragment so the new card is immediately visible.
// The new card's ID is sent in the X-New-Card-ID header so the client can open
// the edit modal without needing a separate round-trip.
func (s *Server) handleUIDuplicateCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	c, err := s.reg.DuplicateCard(id, r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("X-New-Card-ID", c.ID)
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// DELETE /ui/boards/{id}/lanes/{lane} → returns empty string (removes lane element)
func (s *Server) handleUIRemoveLane(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.RemoveLane(id, laneParam(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	// Return 200 OK with empty body so HTMX outerHTML swap removes the lane element.
	// (204 No Content would cause HTMX 2.x to skip the swap per its responseHandling config.)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

// DELETE /ui/boards/{id}/cards/{cardId} → returns empty string (removes card)
func (s *Server) handleUIDeleteCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.DeleteCard(id, r.PathValue("cardId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
}

// POST /ui/boards/{id}/cards/{cardId}/comment → returns comment HTML fragment
func (s *Server) handleUIAddComment(w http.ResponseWriter, r *http.Request) {
	id     := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
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
	cm, err := s.reg.AddComment(id, cardID, req.Author, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CommentItem(*cm, false).Render(r.Context(), w)
}

// PATCH /ui/boards/{id}/cards/{cardId}/comments/{commentId} → returns updated comment HTML fragment
func (s *Server) handleUIUpdateComment(w http.ResponseWriter, r *http.Request) {
	id        := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	cardID    := r.PathValue("cardId")
	commentID := r.PathValue("commentId")
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cm, err := s.reg.UpdateComment(id, cardID, commentID, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CommentItem(*cm, false).Render(r.Context(), w)
}

// PATCH /ui/boards/{id}/cards/{cardId} → returns updated card HTML fragment
func (s *Server) handleUIUpdateCard(w http.ResponseWriter, r *http.Request) {
	id     := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
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
	c, err := s.reg.UpdateCard(id, cardID, service.CardUpdate{
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
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.CardItem(c, labels, id, s.token, s.reg.IsViewBoard(id), false).Render(r.Context(), w)
}

// GET /ui/modals/add-board
func (s *Server) handleUIAddBoardModal(w http.ResponseWriter, r *http.Request) {
	var regularBoards []ui.BoardData
	for _, bi := range s.reg.Boards() {
		if bi.IsView {
			continue
		}
		b, err := s.reg.GetBoard(bi.ID)
		if err != nil {
			continue
		}
		regularBoards = append(regularBoards, ui.BoardData{ID: bi.ID, Name: b.Name})
	}
	currentBoardID := currentRegularBoardID(r, s.reg)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.AddBoardModal(s.token, regularBoards, currentBoardID).Render(r.Context(), w)
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

// GET /ui/modals/board-settings/{id}
func (s *Server) handleUIBoardSettingsModal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var currentColor string
	if s.reg.IsViewBoard(id) {
		vb, _, err := s.reg.GetViewBoard(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		currentColor = vb.Color
	} else {
		b, err := s.reg.GetBoard(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		currentColor = b.Color
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardSettingsModal(id, currentColor, s.token).Render(r.Context(), w)
}

// PATCH /ui/boards/{id}/color
func (s *Server) handleUIUpdateBoardColor(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	var req struct {
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.reg.UpdateBoardColor(id, req.Color); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardPage(data).Render(r.Context(), w)
}

// GET /ui/modals/manage-labels/{boardId}
func (s *Server) handleUIManageLabelsModal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("boardId")
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(id, labels, s.token).Render(r.Context(), w)
}

// GET /ui/boards/{id}/label-picker
func (s *Server) handleUILabelPicker(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.LabelPickerFragment(labels).Render(r.Context(), w)
}

// DELETE /ui/boards/{id}/labels/{labelId}
func (s *Server) handleUIDeleteLabel(w http.ResponseWriter, r *http.Request) {
	id      := boardID(r)
	labelID := r.PathValue("labelId")
	if err := s.reg.RemoveLabel(id, labelID); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(id, labels, s.token).Render(r.Context(), w)
}

// PATCH /ui/boards/{id}/labels/{labelId}
func (s *Server) handleUIRenameLabel(w http.ResponseWriter, r *http.Request) {
	id      := boardID(r)
	labelID := r.PathValue("labelId")
	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.reg.UpdateLabel(id, labelID, service.LabelUpdate{
		Name:  req.Name,
		Color: req.Color,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ManageLabelsModal(id, labels, s.token).Render(r.Context(), w)
}

// GET /ui/modals/archive-view-board/{id}
// Returns the archive view board confirmation dialog as an HTML fragment.
func (s *Server) handleUIArchiveViewBoardModal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vb, parent, err := s.reg.GetViewBoard(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	filterLabelName := ""
	if lbl, ok := bankan.FindLabelByID(parent.Labels, vb.FilterLabel); ok {
		filterLabelName = lbl.Name
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ArchiveViewBoardDialog(id, vb.Name, vb.FilterLabel, filterLabelName, s.token).Render(r.Context(), w)
}

// GET /ui/boards/{id}/labels-fragment → <option> elements for view board creation modal.
func (s *Server) handleUIBoardLabelsFragment(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	labels, _ := s.reg.ListLabels(id)
	w.Header().Set("Content-Type", "text/html")
	_ = ui.ViewBoardFilterLabelOptions(labels).Render(r.Context(), w)
}

// POST /ui/boards/{id}/sync → sync view board with parent and return updated board view.
func (s *Server) handleUISyncViewBoard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.SyncViewBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	data, err := s.buildBoardPage(id, showArchivedFromRequest(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	_ = ui.BoardViewFragment(data).Render(r.Context(), w)
}

// GET /ui/markdown-hints → static markdown syntax cheatsheet modal fragment.
func (s *Server) handleUIMarkdownHints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_ = ui.MarkdownHintsModal().Render(r.Context(), w)
}

// POST /ui/boards/{id}/hide → hide a board from the tab bar.
// Returns 200 + {"navigate_to":"<id>"} when the hidden board was the active one,
// or 204 otherwise.
func (s *Server) handleUIHideBoard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if err := s.reg.HideBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	// Detect whether the caller is currently viewing the hidden board.
	currentURL := r.Header.Get("HX-Current-URL")
	var activeBoardID string
	if u, err := url.Parse(currentURL); err == nil {
		activeBoardID = path.Base(u.Path)
	}
	if activeBoardID == id {
		// Navigate to the first non-hidden, non-archived board.
		for _, bi := range s.reg.Boards() {
			if bi.ID != id && !s.reg.IsHiddenBoard(bi.ID) && !s.reg.IsArchivedViewBoard(bi.ID) {
				writeJSON(w, http.StatusOK, map[string]string{"navigate_to": bi.ID})
				return
			}
		}
		// No visible board left — client navigates to root, which shows the empty state.
		writeJSON(w, http.StatusOK, map[string]string{"navigate_to": ""})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /ui/boards/{id}/show → restore a hidden board to the tab bar.
func (s *Server) handleUIShowBoard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if err := s.reg.ShowBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
