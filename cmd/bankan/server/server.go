// Package server implements the HTTP REST API and HTMX UI for bankan.
// It uses only Go's standard library net/http for routing.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/thekondor/bankan/internal/service"
)

const apiPrefix = "/api/v1"

// Config holds the server configuration.
type Config struct {
	Bind    string // bind address, default "127.0.0.1"
	Port    int    // port, default 8080
	Token   string // pre-set token; if empty, one is generated
	NoToken bool   // disable token protection
}

// Server is the HTTP server holding multiple workspace registries.
type Server struct {
	workspaces []*service.Workspace
	cfg        Config
	token      string
	mux        *http.ServeMux
	logger     *log.Logger
}

// New creates a new Server. If cfg.Token is empty and !cfg.NoToken, a random
// 32-byte hex token is generated.
func New(workspaces []*service.Workspace, cfg Config, logger *log.Logger) (*Server, error) {
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	token := cfg.Token
	if !cfg.NoToken && token == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate token: %w", err)
		}
		token = hex.EncodeToString(b)
	}

	s := &Server{
		workspaces: workspaces,
		cfg:        cfg,
		token:      token,
		mux:        http.NewServeMux(),
		logger:     logger,
	}
	s.registerRoutes()
	return s, nil
}

// Token returns the protection token (empty when NoToken is set).
func (s *Server) Token() string { return s.token }

// Addr returns the listen address string.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.cfg.Bind, s.cfg.Port)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// workspaceByID looks up a workspace by its slug ID. Returns nil if not found.
func (s *Server) workspaceByID(id string) *service.Workspace {
	for _, ws := range s.workspaces {
		if ws.ID == id {
			return ws
		}
	}
	return nil
}

// firstWorkspace returns the first workspace, or nil if there are none.
func (s *Server) firstWorkspace() *service.Workspace {
	if len(s.workspaces) == 0 {
		return nil
	}
	return s.workspaces[0]
}

// requireWorkspace extracts {ws} from the URL path, looks up the workspace,
// and writes a 404 if not found. Returns nil on failure.
func (s *Server) requireWorkspace(w http.ResponseWriter, r *http.Request) *service.Workspace {
	id := r.PathValue("ws")
	ws := s.workspaceByID(id)
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace not found: "+id)
		return nil
	}
	return ws
}

// registerRoutes wires all HTTP routes.
func (s *Server) registerRoutes() {
	mux := s.mux

	// ── REST API ───────────────────────────────────────────────────────────────
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		return s.tokenMiddleware(s.loggingMiddleware(h))
	}
	get := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, s.loggingMiddleware(h))
	}
	mut := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, wrap(h))
	}

	// Workspaces
	get("GET "+apiPrefix+"/workspaces", s.handleListWorkspaces)

	// Boards (workspace-scoped)
	get("GET "+apiPrefix+"/workspaces/{ws}/boards", s.handleListBoards)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards", s.handleInitBoard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/reorder", s.handleReorderBoards)
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}", s.handleGetBoard)

	// View boards
	mut("POST "+apiPrefix+"/workspaces/{ws}/view-boards", s.handleInitViewBoard)

	// Lanes
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}/lanes", s.handleListLanes)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/lanes", s.handleAddLane)
	mut("PATCH "+apiPrefix+"/workspaces/{ws}/boards/{id}/lanes/{lane}", s.handleRenameLane)
	mut("DELETE "+apiPrefix+"/workspaces/{ws}/boards/{id}/lanes/{lane}", s.handleRemoveLane)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/lanes/reorder", s.handleReorderLanes)

	// Cards
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards", s.handleListCards)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards", s.handleAddCard)
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}", s.handleGetCard)
	mut("PATCH "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}", s.handleUpdateCard)
	mut("DELETE "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}", s.handleDeleteCard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/move", s.handleMoveCard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/reorder", s.handleReorderCard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/archive", s.handleArchiveCard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/restore", s.handleRestoreCard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/duplicate", s.handleDuplicateCard)

	// Comments
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/comments", s.handleListComments)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/comments", s.handleAddComment)
	mut("PATCH "+apiPrefix+"/workspaces/{ws}/boards/{id}/cards/{cardId}/comments/{commentId}", s.handleUpdateComment)

	// Labels
	get("GET "+apiPrefix+"/workspaces/{ws}/boards/{id}/labels", s.handleListLabels)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/labels", s.handleAddLabel)
	mut("PATCH "+apiPrefix+"/workspaces/{ws}/boards/{id}/labels/{labelId}", s.handleUpdateLabel)
	mut("DELETE "+apiPrefix+"/workspaces/{ws}/boards/{id}/labels/{labelId}", s.handleRemoveLabel)

	// View board operations
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/sync", s.handleSyncViewBoard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/archive", s.handleArchiveViewBoard)

	// Board visibility
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/hide", s.handleAPIHideBoard)
	mut("POST "+apiPrefix+"/workspaces/{ws}/boards/{id}/show", s.handleAPIShowBoard)

	// ── HTMX UI ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /", s.handleUIRoot)
	// Workspace root redirect (to first visible board in workspace)
	mux.HandleFunc("GET /ui/workspaces/{ws}", s.handleUIWorkspaceRoot)
	mux.HandleFunc("GET /ui/workspaces/{ws}/boards/{id}", s.handleUIBoard)
	mux.HandleFunc("GET /ui/workspaces/{ws}/boards/{id}/lanes/{lane}/cards", s.handleUILaneCards)
	mux.HandleFunc("GET /ui/modals/card/{id}/boards/{boardId}", s.handleUICardModal)
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards", wrap(s.handleUIAddCard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/lanes", wrap(s.handleUIAddLane))
	mux.HandleFunc("DELETE /ui/workspaces/{ws}/boards/{id}/lanes/{lane}", wrap(s.handleUIRemoveLane))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/labels", wrap(s.handleUIAddLabel))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/move", wrap(s.handleUIMoveCard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/archive", wrap(s.handleUIArchiveCard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/restore", wrap(s.handleUIRestoreCard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/duplicate", wrap(s.handleUIDuplicateCard))
	mux.HandleFunc("DELETE /ui/workspaces/{ws}/boards/{id}/cards/{cardId}", wrap(s.handleUIDeleteCard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/comment", wrap(s.handleUIAddComment))
	mux.HandleFunc("PATCH /ui/workspaces/{ws}/boards/{id}/cards/{cardId}/comments/{commentId}", wrap(s.handleUIUpdateComment))
	mux.HandleFunc("PATCH /ui/workspaces/{ws}/boards/{id}/cards/{cardId}", wrap(s.handleUIUpdateCard))
	mux.HandleFunc("GET /ui/modals/add-board", s.loggingMiddleware(s.handleUIAddBoardModal))
	mux.HandleFunc("GET /ui/modals/manage-labels/{boardId}", s.loggingMiddleware(s.handleUIManageLabelsModal))
	mux.HandleFunc("GET /ui/modals/board-settings/{id}", s.loggingMiddleware(s.handleUIBoardSettingsModal))
	mux.HandleFunc("GET /ui/modals/archive-view-board/{id}", s.loggingMiddleware(s.handleUIArchiveViewBoardModal))
	mux.HandleFunc("GET /ui/modals/delete-label/{boardId}/{labelId}", s.loggingMiddleware(s.handleUIDeleteLabelDialog))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/labels/{labelId}/archive", wrap(s.handleUIArchiveLabel))
	mux.HandleFunc("PATCH /ui/workspaces/{ws}/boards/{id}/color", wrap(s.handleUIUpdateBoardColor))
	mux.HandleFunc("GET /ui/workspaces/{ws}/boards/{id}/label-picker", s.loggingMiddleware(s.handleUILabelPicker))
	mux.HandleFunc("GET /ui/workspaces/{ws}/boards/{id}/labels-fragment", s.loggingMiddleware(s.handleUIBoardLabelsFragment))
	mux.HandleFunc("DELETE /ui/workspaces/{ws}/boards/{id}/labels/{labelId}", wrap(s.handleUIDeleteLabel))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/hide", wrap(s.handleUIHideBoard))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/show", wrap(s.handleUIShowBoard))
	mux.HandleFunc("PATCH /ui/workspaces/{ws}/boards/{id}/labels/{labelId}", wrap(s.handleUIRenameLabel))
	mux.HandleFunc("POST /ui/workspaces/{ws}/boards/{id}/sync", wrap(s.handleUISyncViewBoard))

	// Static assets and utilities
	mux.HandleFunc("GET /ui/markdown-hints", s.loggingMiddleware(s.handleUIMarkdownHints))
	mux.HandleFunc("GET /static/", s.handleStatic)
}

// ─── middleware ───────────────────────────────────────────────────────────────

func (s *Server) tokenMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := r.Header.Get("X-Bankan-Token")
			if got != s.token {
				writeError(w, http.StatusUnauthorized, "missing or invalid X-Bankan-Token header")
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next(lw, r)
		s.logger.Printf("%s %s %d", r.Method, r.URL.Path, lw.status)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// ─── error helpers ────────────────────────────────────────────────────────────

func httpStatusFor(err error) int {
	var notFound *service.ErrNotFound
	var forbidden *service.ErrForbidden
	var conflict *service.ErrConflict
	var badReq *service.ErrBadRequest

	switch {
	case errors.As(err, &notFound):
		return http.StatusNotFound
	case errors.As(err, &forbidden):
		return http.StatusForbidden
	case errors.As(err, &conflict):
		return http.StatusConflict
	case errors.As(err, &badReq):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, msg)
}

func writeServiceError(w http.ResponseWriter, err error) {
	writeError(w, httpStatusFor(err), err.Error())
}

// boardID extracts the {id} path value.
func boardID(r *http.Request) string { return r.PathValue("id") }

// laneParam extracts the {lane} path value.
func laneParam(r *http.Request) string {
	return strings.ReplaceAll(r.PathValue("lane"), "-", " ")
}
