package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	bankan "github.com/thekondor/bankan"
	"github.com/thekondor/bankan/internal/service"
)

// ─── JSON response types ─────────────────────────────────────────────────────

type boardJSON struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Dir       string       `json:"dir"`
	CreatedAt time.Time    `json:"created_at"`
	Labels    []bankan.Label `json:"labels"`
	IsView    bool         `json:"is_view"`
	Body      string       `json:"body,omitempty"`
}

type viewBoardJSON struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Dir         string     `json:"dir"`
	Parent      string     `json:"parent"`
	FilterLabel string     `json:"filter_label"`
	CreatedAt   time.Time  `json:"created_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	IsView      bool       `json:"is_view"`
	Body        string     `json:"body,omitempty"`
}

type laneJSON struct {
	Name  string `json:"name"`
	Dir   string `json:"dir"`
	Order int    `json:"order"`
}

type cardJSON struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Body         string     `json:"body,omitempty"`
	Lane         string     `json:"lane"`
	Labels       []string   `json:"labels"`
	PrimaryLabel string     `json:"primary_label,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	MovedAt      *time.Time `json:"moved_at,omitempty"`
	MovedFrom    string     `json:"moved_from,omitempty"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	ArchivedFrom string     `json:"archived_from,omitempty"`
}

type commentJSON struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func cardToJSON(c *bankan.Card) cardJSON {
	labels := c.Labels
	if labels == nil {
		labels = []string{}
	}
	return cardJSON{
		ID:           c.ID,
		Title:        c.Title,
		Body:         c.Body,
		Lane:         c.Lane,
		Labels:       labels,
		PrimaryLabel: c.PrimaryLabel,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		MovedAt:      c.MovedAt,
		MovedFrom:    c.MovedFrom,
		ArchivedAt:   c.ArchivedAt,
		ArchivedFrom: c.ArchivedFrom,
	}
}

// writeJSON writes a JSON-encoded value with status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// rejectArchivedViewBoard writes a 403 if the board id is an archived view board.
// Returns true when the request was rejected (caller should return immediately).
func (s *Server) rejectArchivedViewBoard(w http.ResponseWriter, id string) bool {
	if s.reg.IsArchivedViewBoard(id) {
		writeError(w, http.StatusForbidden, "archived view board is read-only")
		return true
	}
	return false
}

// ─── Board handlers ────────────────────────────────────────────────────────

// GET /api/v1/boards
func (s *Server) handleListBoards(w http.ResponseWriter, r *http.Request) {
	boards := s.reg.Boards()
	result := make([]map[string]any, 0, len(boards))
	for _, bi := range boards {
		if bi.IsView {
			vb, parent, err := s.reg.GetViewBoard(bi.ID)
			if err != nil {
				continue
			}
			result = append(result, map[string]any{
				"id":           bi.ID,
				"name":         vb.Name,
				"dir":          bi.Dir,
				"is_view":      true,
				"parent":       parent.Name,
				"filter_label": vb.FilterLabel,
				"created_at":   vb.CreatedAt,
				"archived_at":  vb.ArchivedAt,
			})
		} else {
			b, err := s.reg.GetBoard(bi.ID)
			if err != nil {
				continue
			}
			result = append(result, map[string]any{
				"id":         bi.ID,
				"name":       b.Name,
				"dir":        bi.Dir,
				"is_view":    false,
				"created_at": b.CreatedAt,
				"labels":     b.Labels,
			})
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/boards/reorder
func (s *Server) handleReorderBoards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if err := s.reg.ReorderBoards(req.IDs); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards
func (s *Server) handleInitBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	b, err := s.reg.InitBoard(req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// ID is the directory basename (slugified), which may differ from the display name.
	writeJSON(w, http.StatusCreated, boardJSON{
		ID:        filepath.Base(b.Dir),
		Name:      b.Name,
		Dir:       b.Dir,
		CreatedAt: b.CreatedAt,
		Labels:    b.Labels,
	})
}

// POST /api/v1/view-boards
func (s *Server) handleInitViewBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		ParentID      string `json:"parent_id"`
		FilterLabelID string `json:"filter_label_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.ParentID == "" || req.FilterLabelID == "" {
		writeError(w, http.StatusBadRequest, "name, parent_id, and filter_label_id are required")
		return
	}
	vb, err := s.reg.InitViewBoard(req.Name, req.ParentID, req.FilterLabelID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              filepath.Base(vb.Dir),
		"name":            vb.Name,
		"dir":             vb.Dir,
		"parent_id":       req.ParentID,
		"filter_label_id": vb.FilterLabel,
		"is_view":         true,
		"created_at":      vb.CreatedAt,
	})
}

// GET /api/v1/boards/{id}
func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.reg.IsViewBoard(id) {
		vb, parent, err := s.reg.GetViewBoard(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, viewBoardJSON{
			ID:          id,
			Name:        vb.Name,
			Dir:         vb.Dir,
			Parent:      parent.Name,
			FilterLabel: vb.FilterLabel,
			CreatedAt:   vb.CreatedAt,
			ArchivedAt:  vb.ArchivedAt,
			IsView:      true,
			Body:        vb.Body,
		})
		return
	}
	b, err := s.reg.GetBoard(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, boardJSON{
		ID:        id,
		Name:      b.Name,
		Dir:       b.Dir,
		CreatedAt: b.CreatedAt,
		Labels:    b.Labels,
		Body:      b.Body,
	})
}

// ─── Lane handlers ────────────────────────────────────────────────────────

// GET /api/v1/boards/{id}/lanes
func (s *Server) handleListLanes(w http.ResponseWriter, r *http.Request) {
	lanes, err := s.reg.ListLanes(boardID(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result := make([]laneJSON, len(lanes))
	for i, l := range lanes {
		result[i] = laneJSON{Name: l.Name, Dir: l.Dir, Order: l.Order}
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/boards/{id}/lanes
func (s *Server) handleAddLane(w http.ResponseWriter, r *http.Request) {
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
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	l, err := s.reg.AddLane(id, req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, laneJSON{Name: l.Name, Dir: l.Dir, Order: l.Order})
}

// PATCH /api/v1/boards/{id}/lanes/{lane}
func (s *Server) handleRenameLane(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		NewName string `json:"new_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.NewName == "" {
		writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}
	if err := s.reg.RenameLane(id, laneParam(r), req.NewName); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/boards/{id}/lanes/{lane}
func (s *Server) handleRemoveLane(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.RemoveLane(id, laneParam(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/lanes/reorder
func (s *Server) handleReorderLanes(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		Names []string `json:"names"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names is required")
		return
	}
	if err := s.reg.ReorderLanes(id, req.Names); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Card handlers ────────────────────────────────────────────────────────

// GET /api/v1/boards/{id}/cards?lane=&archived=true
func (s *Server) handleListCards(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	laneName := r.URL.Query().Get("lane")
	archived := r.URL.Query().Get("archived") == "true"

	if archived {
		cards, err := s.reg.ListArchivedCards(id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result := make([]cardJSON, len(cards))
		for i, c := range cards {
			result[i] = cardToJSON(c)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if laneName != "" {
		cards, err := s.reg.ListCards(id, laneName)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		result := make([]cardJSON, len(cards))
		for i, c := range cards {
			result[i] = cardToJSON(c)
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	// All lanes.
	all, err := s.reg.ListAllCards(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var result []cardJSON
	lanes, _ := s.reg.ListLanes(id)
	for _, lane := range lanes {
		for _, c := range all[lane.Name] {
			result = append(result, cardToJSON(c))
		}
	}
	if result == nil {
		result = []cardJSON{}
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/boards/{id}/cards
func (s *Server) handleAddCard(w http.ResponseWriter, r *http.Request) {
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
	if req.Lane == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "lane and title are required")
		return
	}
	c, err := s.reg.AddCard(id, req.Lane, req.Title, req.Body, req.LabelIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cardToJSON(c))
}

// GET /api/v1/boards/{id}/cards/{cardId}
func (s *Server) handleGetCard(w http.ResponseWriter, r *http.Request) {
	c, err := s.reg.GetCard(boardID(r), r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cardToJSON(c))
}

// PATCH /api/v1/boards/{id}/cards/{cardId}
func (s *Server) handleUpdateCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
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
	c, err := s.reg.UpdateCard(id, r.PathValue("cardId"), service.CardUpdate{
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
	writeJSON(w, http.StatusOK, cardToJSON(c))
}

// DELETE /api/v1/boards/{id}/cards/{cardId}?force=true
func (s *Server) handleDeleteCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	force := r.URL.Query().Get("force") == "true"
	if !force {
		writeError(w, http.StatusBadRequest, "add ?force=true to confirm permanent deletion")
		return
	}
	if err := s.reg.DeleteCard(id, r.PathValue("cardId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/cards/{cardId}/move
func (s *Server) handleMoveCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		ToLane string `json:"to_lane"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ToLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := s.reg.MoveCard(id, r.PathValue("cardId"), req.ToLane); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/cards/{cardId}/reorder
func (s *Server) handleReorderCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		NewIndex int `json:"new_index"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.NewIndex < 0 {
		writeError(w, http.StatusBadRequest, "new_index must be >= 0")
		return
	}
	if err := s.reg.ReorderCard(id, r.PathValue("cardId"), req.NewIndex); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/cards/{cardId}/archive
func (s *Server) handleArchiveCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.ArchiveCard(id, r.PathValue("cardId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/cards/{cardId}/restore
func (s *Server) handleRestoreCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		ToLane string `json:"to_lane"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ToLane == "" {
		writeError(w, http.StatusBadRequest, "to_lane is required")
		return
	}
	if err := s.reg.RestoreCard(id, r.PathValue("cardId"), req.ToLane); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/cards/{cardId}/duplicate
func (s *Server) handleDuplicateCard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	c, err := s.reg.DuplicateCard(id, r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cardToJSON(c))
}

// ─── Comment handlers ─────────────────────────────────────────────────────

// GET /api/v1/boards/{id}/cards/{cardId}/comments
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.reg.ListComments(boardID(r), r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	result := make([]commentJSON, len(comments))
	for i, cm := range comments {
		result[i] = commentJSON{
			ID:        cm.ID,
			Author:    cm.Author,
			Body:      cm.Body,
			CreatedAt: cm.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /api/v1/boards/{id}/cards/{cardId}/comments
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	cm, err := s.reg.AddComment(id, r.PathValue("cardId"), req.Author, req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, commentJSON{
		ID:        cm.ID,
		Author:    cm.Author,
		Body:      cm.Body,
		CreatedAt: cm.CreatedAt,
	})
}

// PATCH /api/v1/boards/{id}/cards/{cardId}/comments/{commentId}
func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	cm, err := s.reg.UpdateComment(boardID(r), r.PathValue("cardId"), r.PathValue("commentId"), req.Body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, commentJSON{
		ID:        cm.ID,
		Author:    cm.Author,
		Body:      cm.Body,
		CreatedAt: cm.CreatedAt,
	})
}

// ─── Label handlers ────────────────────────────────────────────────────────

// GET /api/v1/boards/{id}/labels
func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.reg.ListLabels(boardID(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if labels == nil {
		labels = []bankan.Label{}
	}
	writeJSON(w, http.StatusOK, labels)
}

// POST /api/v1/boards/{id}/labels
func (s *Server) handleAddLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Color == "" {
		writeError(w, http.StatusBadRequest, "name and color are required")
		return
	}
	l, err := s.reg.AddLabel(boardID(r), req.Name, req.Color)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// PATCH /api/v1/boards/{id}/labels/{labelId}
func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	l, err := s.reg.UpdateLabel(boardID(r), r.PathValue("labelId"), service.LabelUpdate{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// DELETE /api/v1/boards/{id}/labels/{labelId}
func (s *Server) handleRemoveLabel(w http.ResponseWriter, r *http.Request) {
	if err := s.reg.RemoveLabel(boardID(r), r.PathValue("labelId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── View board handlers ─────────────────────────────────────────────────────

// POST /api/v1/boards/{id}/sync
func (s *Server) handleSyncViewBoard(w http.ResponseWriter, r *http.Request) {
	id := boardID(r)
	if s.rejectArchivedViewBoard(w, id) {
		return
	}
	if err := s.reg.SyncViewBoard(id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/archive
func (s *Server) handleArchiveViewBoard(w http.ResponseWriter, r *http.Request) {
	// Body is optional; decode leniently (no DisallowUnknownFields).
	var req struct {
		ArchiveLabel bool `json:"archive_label"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.reg.ArchiveViewBoard(boardID(r), req.ArchiveLabel); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/hide
func (s *Server) handleAPIHideBoard(w http.ResponseWriter, r *http.Request) {
	if err := s.reg.HideBoard(boardID(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/boards/{id}/show
func (s *Server) handleAPIShowBoard(w http.ResponseWriter, r *http.Request) {
	if err := s.reg.ShowBoard(boardID(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
