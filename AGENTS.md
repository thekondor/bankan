# AGENTS.md — Context and Extension Guide for LLM Agents

This file is the primary reference for any AI agent working on this codebase.
Read it before making any changes. It describes what the project is, how its
internals are structured, what invariants must be preserved, and how to safely
extend it.

---

## Project purpose

`bankan` is a **local-first kanban board manager**. All state is stored in plain
markdown files with YAML frontmatter — no database, no daemon, no network. A
board is a directory; copying or moving it is a complete backup.

The primary use-case is keeping boards inside a software project and tracking
them with git.

The module is `github.com/thekondor/bankan`, written in Go 1.26.

## Instructions

- Always ask questions in case of concerns, do not make assumptions
- Always update or add new unit and integration tests for new logic
- Always keep API for CLI and REST endpoint equal.

---

## Repository layout

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
└── AGENTS.md                     # This file
```

The root package `bankan` is the library. `internal/service` is the shared
orchestration layer. The CLI (`cmd/bankan/main.go`) and HTTP server
(`cmd/bankan/server`) are the two consumers — neither is imported by the
library.

---

## File format — the single most important thing to understand

### Board directory

A directory is a board if and only if it contains `board.md`. `IsBoard(dir)`
checks this.

```
<board-dir>/
├── board.md
├── 01-backlog/
│   ├── 001-ab12c-fix-login.md
│   └── 001-ab12c-fix-login.comments.md
├── 02-in-progress/
└── _archive/
    ├── ab12c-fix-login.md          ← no order prefix in archive
    └── ab12c-fix-login.comments.md
```

### Board `board.md`

```yaml
---
name: My Board
order: 1
hidden: true
created_at: 2026-05-11T10:00:00Z
labels:
  - id: ab12c
    name: Bug
    color: "#ef4444"
---
```

The `hidden` field (`bool`, `omitempty`) hides the board from the UI tab bar
when `true`. It is set by `HideBoard` and cleared by `ShowBoard`. Absent or
`false` means visible.

### Lane directory naming

```
NN-<slug>
```

- `NN` is a two-digit zero-padded integer starting at `01`.
- `slug` is the display name lowercased, spaces → hyphens, special chars dropped.
- `_archive` is reserved and never treated as a lane.
- `parseLaneDir(base)` is the canonical parser. It returns `(order int, name string, ok bool)`.
- `ReadLanes` ignores any directory whose name does not match `laneNameRe`.

### Card file naming (in a lane)

```
NNN-<id>-<slug>.md
```

- `NNN` is a three-digit zero-padded order prefix (1–999).
- `id` is exactly 5 lowercase alphanumeric characters (`[a-z0-9]{5}`).
- `slug` is derived from the title at creation, **frozen forever** (slug never
  changes even if the title is later edited).
- `slug` must not contain `.` — the regex `cardFileRe` uses `[^.]+` for the
  slug group specifically to exclude `.comments.md` files from matching as cards.

### Card file naming (in `_archive/`)

```
<id>-<slug>.md
```

No order prefix. Archive files are matched by `archCardRe`:
`^([a-z0-9]{5})-([^.]+)\.md$`

### Comments file naming

The comments file shares the base name of its card but with `.comments.md`:

```
001-ab12c-fix-login.md          → 001-ab12c-fix-login.comments.md
ab12c-fix-login.md (archive)   → ab12c-fix-login.comments.md
```

`commentFilename(cardBase string) string` handles this. Comments files are
**always co-located** with their card and are moved/renamed atomically with it.

### Comments file content

```markdown
# Comments: <card-id>

## <comment-id> · <RFC3339> · <author>

Comment body in **markdown**.

---

## <comment-id2> · <RFC3339> · <author2>

Second comment.
```

The parser (`parseComments`) is **line-oriented**: it scans for H2 lines
matching `## <id> · <ts> · <author>` and accumulates subsequent lines as body.
`---` lines are silently discarded (they are decorative separators). A `---`
within a comment body will therefore be swallowed — authors should use `***`
or `- - -` for in-body horizontal rules.

### View board directory

A directory is a view board if and only if it contains `view.md`.
`IsViewBoard(dir)` checks this. A directory cannot be both a board and a view
board at the same time.

```
<view-dir>/
├── view.md                                  ← sentinel file
├── 01-backlog/
│   ├── 001-ab12c-fix-login.md               ← stub (card_id only)
│   └── 002-xk9p2-add-oauth.md
├── 02-in-progress/
└── 03-sprint-icebox/                        ← view-only lane
    └── 001-mn3rs-tech-debt.md
```

The `_archive/` directory exists for structural consistency but is not used by
view boards (cards are never actually archived in the view layer).

### View board stub file naming

Stubs in view lane directories use the **same filename pattern** as regular cards:
`NNN-<id>-<slug>.md`. The slug is derived from the card title at stub creation
time. The content is minimal:

```yaml
---
card_id: ab12c
---
```

### View board sync semantics

Stubs **do not self-update** when a card moves in the parent board. A stub's
lane position is updated only when `SyncViewBoard` is called explicitly (via
`bankan board view sync --board <view>` or `POST /api/v1/boards/{id}/sync`).

`SyncViewBoard` applies four passes in order:

1. **Want set** — collect all parent cards that carry `FilterLabel`.
2. **Add missing** — for each want-set card with no stub, create a stub in the
   matching view lane (by lane name). Fallback: first view lane if none matches;
   skip if the view has no lanes.
3. **Relocate misplaced** — for each existing stub whose parent card has moved
   to a different lane: if the parent's new lane has a matching view lane, move
   the stub there. If there is no matching view lane, the stub stays put.
4. **Remove orphans** — delete stubs for cards that no longer carry `FilterLabel`.

**Consequence for callers**: when you move a card in the parent board (e.g. via
`MoveCard`), view board stubs are **not** updated automatically. Code that
moves parent-board cards must document that a subsequent `SyncViewBoard` call
is required to keep view stubs consistent. Do not assume stubs track the
parent's current lane without an explicit sync.

### View board `view.md`

```yaml
---
name: Sprint 1 View
parent: /absolute/path/to/parent/board
filter_label: ab12c
created_at: 2026-05-11T10:00:00Z
archived_at: null
hidden: true
---
```

The `filter_label` field is **immutable** after creation. Never write code that
changes it after `InitViewBoard` returns.

The `hidden` field (`bool`, `omitempty`) is present on both `board.md` and
`view.md`. When `true`, the board is hidden from the UI tab bar and placed in
the overflow dropdown. It is set by `HideBoard`/`HideViewBoard` and cleared by
`ShowBoard`/`ShowViewBoard`. Hidden boards remain fully readable and mutable —
hiding is purely a display preference.

---

## Key invariants — never break these

1. **Card IDs are unique within a board** (across all lanes and `_archive`).
   `collectCardIDs(b)` scans everything before generating a new ID. Do not
   skip this scan when adding any creation path.

2. **Label IDs and names are unique per board** (case-insensitive for names).
   Always pass the candidate slice through `ValidateLabels` before writing.

3. **`archived_at` is non-nil if and only if the card is in `_archive/`.**
   `ArchiveCard` sets it, `RestoreCard` clears it. No other code should touch
   these fields.

4. **`updated_at` is set by `WriteCard` on every call**, not by callers.
   Callers should mutate `c.Body`, `c.Title`, etc., then call `WriteCard(c)`.
   Do not set `UpdatedAt` manually before calling `WriteCard`.

5. **The card slug is frozen at creation.** `AddCard` derives the slug from
   the initial title via `slugify`. Subsequent `WriteCard` calls never rename
   the file; they overwrite it in place.

6. **Comments files travel with their card.** Every function that moves,
   archives, restores, or deletes a card file must also handle the co-located
   `.comments.md` file. See `MoveCard`, `ArchiveCard`, `RestoreCard`,
   `DeleteCard` for the established pattern.

7. **Card order prefix is append-only for new cards.** New cards always get
   `max(existing prefix) + 1`. `ReorderCard` and `ReorderCardAmongLabeled`
   reorder existing cards by redistributing the current set of prefixes — they
   never introduce new values. The prefix can grow past `999` only if a lane
   has more than 999 cards; that is unsupported and would require a
   normalisation pass.

8. **`ViewBoard.FilterLabel` is immutable after `InitViewBoard`.** Never write
   code that changes it. The CLI enforces this by blocking `label remove` of
   the filter label when operating on a view board.

9. **View board stubs never duplicate card data.** All card fields (title, body,
   labels, comments) are always read from the parent board's actual files via
   `ResolveViewCard`. Stubs contain only `card_id`.

10. **View board lane uniqueness spans parent + view.** `AddViewLane` must
    check for name conflicts in both `ReadLanes(vb.Dir)` and `ReadLanes(parent.Dir)`.
    This invariant prevents silent divergence where a "shared" lane in the view
    appears view-only.

11. **`Board.Order` / `ViewBoard.Order` is the sole source of tab display order.**
    The `order` field (int, `omitempty`) in `board.md` / `view.md` controls the
    tab bar position. `order == 0` means "unset" and those boards sort last
    (alphabetically among themselves). `service.Registry` caches the order at
    registration time and re-reads it on `ReorderBoards`. Never derive display
    order from directory names or creation timestamps. New boards created via
    `service.Registry.InitBoard` / `InitViewBoard` automatically get
    `order = max(existing) + 1` so they appear at the end of the tab bar.

12. **`Board.Hidden` / `ViewBoard.Hidden` is purely a display preference — it
    never affects reads or mutations.** When `hidden: true`, the board is
    excluded from the UI tab bar and placed in the overflow dropdown; it remains
    fully readable and mutable via CLI and REST. `HideBoard`/`HideViewBoard` set
    the flag; `ShowBoard`/`ShowViewBoard` clear it. A board cannot be both
    `hidden:true` and `archived_at != nil` at the same time — hide takes priority
    in `buildBoardPage`. Never treat hidden boards as read-only.

---

## Go style conventions for this project

- **Flat structs, no interfaces, no dependency injection.** Functions accept
  and return concrete `*Board`, `Lane`, `*Card`, `Comment` values.
- **Package `bankan` is the only library package.** Do not create sub-packages
  inside the root. New library concerns go in new `.go` files in the root.
  `internal/service` and `cmd/bankan/server` are _not_ library packages —
  they are implementation packages and may import `cobra`, `net/http`, etc.
- **Error wrapping with `fmt.Errorf("context: %w", err)`.** All errors are
  wrapped with a context prefix matching the operation name, e.g.
  `"add card: ..."`, `"move card: ..."`.
- **`os.IsNotExist` for missing files** — several functions (e.g. `ReadComments`,
  `ListArchivedCards`) treat a missing file as an empty-not-an-error result.
  Follow this pattern when reading optional files.
- **`t.TempDir()` in every test.** No global state, no fixed paths, no cleanup
  functions. Tests are safe to run in parallel.
- **No mocking.** Tests use real filesystem I/O via `t.TempDir()`. This keeps
  tests honest about the file format.
- **File permissions**: directories `0o755`, files `0o644`.

---

## How to add a new field to `Card`

1. Add the field to `Card` struct in `card.go` with a YAML tag.
   - Fields that are optional/nullable use `*time.Time` or `omitempty`.
   - Runtime-only fields (not persisted) get `yaml:"-"`.
2. If the field needs to be cleared on a lifecycle transition (e.g. archive,
   restore), update the relevant function in `card.go`.
3. `WriteCard` serializes the whole struct — no further serialization changes
   needed.
4. Add a unit test in `card_test.go` that writes and reads back the new field.
5. If the field is user-facing, add a CLI flag in `cmd/bankan/main.go`.

---

## How to add a new lifecycle operation on a card

Pattern (copy from `MoveCard` or `ArchiveCard`):

1. Find the card: `FindCard(b, id, searchArchive)`.
2. Compute the destination path.
3. Update the card struct fields in memory.
4. Serialize and write to the **new** path with `os.WriteFile`.
5. Move the `.comments.md` file if present (check with `os.Stat` first).
6. Remove the old card file with `os.Remove`.
7. Update `c.FilePath` to the new path.
8. Write tests covering: happy path, comments file migration, error cases.

Never use `os.Rename` for card files across directories on different filesystems.
The current pattern (write new → rename/move comments → remove old) is
intentionally explicit.

---

## How to add a new CLI command

All commands live in `cmd/bankan/main.go`. CLI commands use `service.Registry`
(via `resolveReg`) rather than calling the `bankan` library directly.

1. Write a `newXxxCmd() *cobra.Command` constructor function.
2. Resolve the board context with `resolveReg(boardDir)` — returns
   `(*service.Registry, string, error)` where the string is the board ID.
3. Use `--board` flag consistently: `cmd.Flags().StringVar(&boardDir, "board", "", "Path to board directory")`.
4. For author fields: `author = gitUsername()` as the fallback.
5. Register the new command in `root.AddCommand(...)` inside `main()`.
6. Error output: return the error from `RunE`; cobra prints it. Do not call
   `os.Exit` or `log.Fatal` inside command functions.

Commands that do not operate on a board (e.g. `ai-skill`) skip step 2 and 3.
If the command needs embedded file resources, create a sub-package under
`cmd/bankan/<name>/` with its own `//go:embed` declaration (see
`cmd/bankan/skill/skill.go` as the reference implementation).

Commands that operate across **multiple boards** (e.g. `board reorder`) must
not use `resolveReg` (which creates a single-board registry). Instead, accept
a `--root` flag pointing to the container directory and build a multi-board
registry directly:
```go
reg, err := service.NewRegistry([]string{rootDir}, rootDir)
```
See `newBoardReorderCmd` as the reference implementation.

---

## How to add a new REST API endpoint

1. Add a method `func (s *Server) handleXxx(w http.ResponseWriter, r *http.Request)` in
   `cmd/bankan/server/handlers_api.go`.
2. Register the route in `registerRoutes()` in `cmd/bankan/server/server.go` using
   Go 1.22 method+path syntax. Use the local `mut()` helper for token-protected
   mutating routes: `mut("POST "+apiPrefix+"/boards/{id}/xxx", s.handleXxx)`.
   For UI (HTMX) routes use `mux.HandleFunc("POST /ui/boards/{id}/xxx", wrap(s.handleXxx))`.
3. Add a corresponding method to `service.Registry` in `internal/service/` if
   needed.
4. Add a test in `cmd/bankan/server/server_test.go` using `newTestServer(t)`.
5. If the operation has a UI counterpart, add a handler in `handlers_ui.go` and
   a corresponding JS function in `cmd/bankan/ui/static/app.js`.

---

## How to update UI templates

Templates live in `cmd/bankan/ui/*.templ`. After any change run:

```bash
templ generate ./cmd/bankan/ui/
```

This regenerates the `*_templ.go` files. Both the `.templ` source and the
generated `*_templ.go` files are committed to the repository.

Static assets (CSS, JS) are in `cmd/bankan/ui/static/` and embedded via
`//go:embed static` in `cmd/bankan/ui/static.go`. No generation step is needed
for static assets.

---

## How to add a new view board operation

1. Add the function to the appropriate view file (`viewboard.go`,
   `viewcard.go`, or `viewlane.go`).
2. Functions accept `(vb *ViewBoard, parent *Board, ...)` — always explicit,
   never implicit.
3. All mutations that affect parent card data call existing `WriteCard` or
   `ReadCard` on the parent's file path.
4. All stub mutations (create, move, delete) operate on files under `vb.Dir`.
5. Add unit tests in the corresponding `view*_test.go` file.
6. Add an integration test scenario in `lifecycle_integration_test.go` under
   `TestLifecycle_ViewBoard_*`.
7. Wire CLI in `cmd/bankan/main.go` under the `bc.isView()` branch of the
   relevant command.

---

## How to add a new lane or board-level operation

For lane operations, follow the pattern in `lane.go`:
- Accept `*Board` as the first argument so you have `b.Dir`.
- Call `ReadLanes(b.Dir)` to get the current state.
- Validate (uniqueness, existence) before mutating the filesystem.
- Return the new/modified `Lane` value.

For board-level operations (e.g. a new kind of metadata), add the field to
`Board`, add a function that calls `ReadBoard` + mutates + `WriteBoard`.

---

## How to write tests

### Unit test (testing a single function)

```go
func TestMyFunc_SomeCase(t *testing.T) {
    b := newTestBoard(t)   // helper in board_test.go / lane_test.go
    // or: dir := t.TempDir() then InitBoard(dir, "X")

    result, err := MyFunc(b, ...)
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

Helpers available in `*_test.go` files (package `bankan`):
- `newTestBoard(t)` — creates a board in `t.TempDir()`
- `boardWithLane(t)` — creates a board + one lane named "Backlog"
- `addTestCard(t, b, lane, title)` — adds a card with body "body text"
- `newTestViewBoard(t)` — creates a parent board with a label + one lane, and a view board filtered by that label
- `newViewBoardWithCard(t)` — extends `newTestViewBoard` by adding one labelled card to the parent

### Integration test (package `bankan_test`)

Add to `lifecycle_integration_test.go`. Import as:
```go
import bankan "github.com/thekondor/bankan"
```

Integration tests must:
- Exercise a multi-step workflow (at least 3 operations).
- Verify filesystem state (file existence, file contents via re-read) not just
  return values.
- Name the test `TestLifecycle_<WorkflowName>`.

### Running tests

```bash
go test ./...               # all tests
go test -run TestFoo ./...  # specific test
go test -v -count=1 ./...   # verbose, no cache
```

---

## Known limitations (intentional, not bugs)

- **Card order prefix caps at 999 per lane.** No normalisation pass exists yet.
  A lane with 1000 cards would produce `1000-...` which breaks the 3-digit
  regex. Fix: increase regex to `\d+`.
- **Lane order is append-only.** Reordering lanes requires renaming directories
  and is not yet implemented.
- **`moved_at` / `moved_from` record only the most recent move.** Full move
  history would require either a separate activity log per card or the git log.
- **No concurrent write safety at the library level.** The root `bankan`
  package does no file locking. `internal/service.Registry` adds per-board
  `sync.RWMutex` protection for the HTTP server, but direct use of the library
  from multiple goroutines without external locking can corrupt state.
- **`---` in comment body is swallowed by the parser.** The line-oriented
  parser skips any line that is exactly `---`. Authors needing an HR inside a
  comment body must use `***` or `- - -`.
- **Label IDs on cards are not referential-integrity-checked on read.** If a
  label is removed from the board but still referenced by a card, `ReadCard`
  returns the card with the stale ID silently. Validation is only enforced at
  `AddCard` time.

---

## Things that are explicitly out of scope (do not add)

- Network sync, conflict resolution, or cloud storage.
- Authentication or multi-user access control (the HTTP server uses a
  single static token; there is no per-user identity).
- Binary attachments — the format is text-only by design.
- A database or index file — all queries scan the filesystem.
- Sub-packages inside the root `bankan` package — keep the library flat.

---

## Dependency summary

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v3` | YAML frontmatter marshal/unmarshal |
| `github.com/spf13/cobra` | CLI subcommand routing |
| `github.com/stretchr/testify` | Test assertions (`assert`, `require`) |
| `github.com/a-h/templ` | Type-safe HTML templates (UI layer only) |

Do not add new dependencies without a strong reason. The library (`package bankan`)
must not import `cobra`, `templ`, or `net/http` — those dependencies belong
only in `cmd/bankan` and `internal/service`.
