# Repository Layout

```
bankan.d/
├── id.go                         # ID generation
├── frontmatter.go                # YAML frontmatter codec
├── label.go                      # Label type + board-scoped validation
├── board.go                      # Board type + board.md I/O + label mutations + display order
├── lane.go                       # Lane type + directory operations
├── card.go                       # Card type + full lifecycle
├── comment.go                    # Comment type + comments file I/O
├── viewboard.go                  # ViewBoard type + view.md I/O + lifecycle + display order
├── viewcard.go                   # ViewCardStub type + view-specific card operations
├── viewlane.go                   # View-specific lane ops (cross-board uniqueness)
├── *_test.go                     # Unit tests (package bankan)
├── lifecycle_integration_test.go # Integration tests (package bankan_test)
├── internal/
│   └── service/                  # Shared service layer (CLI + HTTP server)
│       ├── registry.go           # Registry: per-board mutexes + board lookup
│       ├── board.go              # Board / view board service operations
│       ├── lane.go               # Lane service operations
│       ├── card.go               # Card operations + CardUpdate struct + CardSearchResult + SearchCard
│       ├── comment.go            # Comment service operations
│       ├── label.go              # Label operations + LabelUpdate struct
│       ├── errors.go             # NotFoundError, ConflictError, ValidationError, ForbiddenError
│       └── service_test.go       # unit tests
├── cmd/bankan/
│   ├── main.go                   # CLI binary (cobra) + newServeCmd() + newAISkillCmd()
│   ├── server/
│   │   ├── server.go             # HTTP server, routing, token middleware
│   │   ├── handlers_api.go       # REST JSON handlers
│   │   ├── handlers_ui.go        # HTMX HTML fragment handlers + static serving
│   │   └── server_test.go        # Integration tests (37 tests)
│   ├── skill/
│   │   ├── skill.go              # //go:embed templates → skill.TemplateFS
│   │   └── templates/
│   │       └── bankan.md.tmpl    # Claude Code compatible skill template
│   └── ui/
│       ├── layout.templ / layout_templ.go
│       ├── board.templ  / board_templ.go
│       ├── card.templ   / card_templ.go
│       ├── types.go              # BoardPageData, CardDetailData, LaneWithCards
│       ├── static.go             # //go:embed static → ui.StaticFS
│       └── static/
│           ├── style.css
│           ├── app.js
│           ├── htmx.min.js
│           └── sortable.min.js
├── ARCHITECTURE.md               # Human-readable architecture doc
└── AGENTS.md                     # Entry point for LLM agents
```

The root package `bankan` is the library. `internal/service` is the shared
orchestration layer. The CLI (`cmd/bankan/main.go`) and HTTP server
(`cmd/bankan/server`) are the two consumers — neither is imported by the
library.
