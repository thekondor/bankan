# Architecture

## Overview

`bankan` is a local-first kanban board manager. All state is stored in plain
markdown files and readable YAML frontmatter — no database, no daemon, no
network. A board is just a directory; moving or copying it is a full backup.

The primary use-case is embedding one or more boards inside a software project
directory and tracking them with git alongside the source code.

---

## File Format Specification

### Board directory layout

```
<board-dir>/
├── board.md                          # Board metadata + labels
├── 01-backlog/                       # Lane (NN-<slug> naming)
│   ├── 001-ab12c-fix-login-bug.md   # Card file
│   └── 001-ab12c-fix-login-bug.comments.md
├── 02-in-progress/
│   └── 001-xk9p2-add-oauth.md
├── 03-done/
└── _archive/                         # Archived cards (flat, no order prefix)
    ├── ab12c-fix-login-bug.md
    └── ab12c-fix-login-bug.comments.md
```

A directory is recognised as a board by the presence of `board.md` in its root.

---

### `board.md`

YAML frontmatter followed by an optional markdown description.

```markdown
---
name: My Project Board
order: 1
created_at: 2026-01-01T10:00:00Z
labels:
  - id: ab1de
    name: Bug
    color: "#ef4444"
  - id: fg2hi
    name: Feature
    color: "#3b82f6"
---

Optional board description in **markdown**.
```

| Field       | Type     | Notes                                   |
|-------------|----------|-----------------------------------------|
| `name`      | string   | Display name of the board               |
| `order`     | int?     | Tab bar position (1-based); `0`/absent means unordered, sorts last |
| `hidden`    | bool?    | When `true`, excluded from tab bar and placed in overflow dropdown |
| `created_at`| RFC3339  | Set once at `board init`                |
| `labels`    | list     | All labels for this board; unique by ID and name |

Labels are the only board-scoped enumeration. They are referenced from cards by
their `id`.

---

### Lane directories

Lane directories use a two-digit numeric prefix for ordering:

```
NN-<slug>/
```

Examples: `01-backlog/`, `02-in-progress/`, `03-done/`

- `NN` starts at `01` and increments by 1 for each new lane appended.
- `slug` is the display name lowercased with spaces and special characters
  replaced by hyphens.
- The display name is stored implicitly in the directory name; no separate config
  file is needed.
- Lane order is the `NN` prefix value, not filesystem mtime.

The special directory `_archive/` is always present but never treated as a lane.

---

### Card files

Card filenames inside a lane follow:

```
NNN-<id>-<slug>.md
```

Example: `001-ab12c-fix-login-bug.md`

- `NNN` is a three-digit zero-padded order prefix (max 999 per lane before you
  need to consider normalisation, which is a future concern).
- `id` is a 5-character random lowercase alphanumeric string.
- `slug` is derived from the title at creation time and never changes afterward.

The card body is a YAML-frontmatter markdown file:

```markdown
---
id: ab12c
title: Fix login bug
created_at: 2026-01-01T10:00:00Z
updated_at: 2026-01-02T15:30:00Z
moved_at: 2026-01-02T14:00:00Z
moved_from: backlog
labels:
  - ab1de
---

Card body in **markdown**.

- [ ] Check form validation
- [ ] Test on mobile
```

| Field          | Type      | Notes                                              |
|----------------|-----------|----------------------------------------------------|
| `id`           | string    | 5-char random alphanumeric; stable for card lifetime |
| `title`        | string    | Display title                                      |
| `created_at`   | RFC3339   | Set once at creation                               |
| `updated_at`   | RFC3339   | Updated on every `WriteCard` call                  |
| `moved_at`     | RFC3339?  | Set when card moves between lanes                  |
| `moved_from`   | string?   | Display name of the previous lane                  |
| `archived_at`  | RFC3339?  | Set when card is archived                          |
| `archived_from`| string?   | Display name of the lane at archive time           |
| `labels`       | []string? | List of label IDs from the board                   |

Fields marked `?` are optional and omitted when not applicable.

For archived cards in `_archive/`, the filename loses its order prefix:

```
<id>-<slug>.md   →   ab12c-fix-login-bug.md
```

---

### Comments files

Each card may have a co-located comments file. The filename is the card's
filename with `.md` replaced by `.comments.md`:

```
001-ab12c-fix-login-bug.md
001-ab12c-fix-login-bug.comments.md
```

Comments files are moved and renamed alongside their parent card file in all
operations (move, archive, restore, delete).

**File format:**

```markdown
# Comments: ab12c

## c1a2b · 2026-01-01T10:00:00Z · alice

First comment with **markdown** support.

---

## d3e4f · 2026-01-02T09:00:00Z · bob

Second comment.
```

Each comment section begins with an H2 heading:

```
## <comment-id> · <RFC3339 timestamp> · <author>
```

Sections are separated by `---` (HR). The parser is line-oriented and does not
rely on block splitting, so markdown HR within a comment body would be
ambiguous; authors should use `- - -` or `***` for body HRs instead.

---

## Package Design

The project is a single Go module (`github.com/thekondor/bankan`) with all
library types in the root package `bankan`, a shared service layer in
`internal/service`, the HTTP server in `cmd/bankan/server`, UI templates and
static assets in `cmd/bankan/ui`, and the CLI binary in `cmd/bankan/`.

```
bankan.d/
├── id.go                  # ID generation
├── frontmatter.go         # YAML frontmatter parse/serialize
├── label.go               # Label type + validation
├── board.go               # Board type + board.md I/O
├── lane.go                # Lane type + directory operations
├── card.go                # Card type + file I/O + lifecycle
├── comment.go             # Comment type + comments file I/O
├── viewboard.go           # ViewBoard type + view.md I/O
├── viewcard.go            # ViewCardStub type + view-specific card operations
├── viewlane.go            # View-specific lane operations (cross-board uniqueness)
├── *_test.go              # Unit tests (same package)
├── lifecycle_integration_test.go  # Integration tests (external package)
├── internal/
│   └── service/           # Shared service layer (used by CLI + HTTP server)
│       ├── registry.go    # Registry: per-board mutexes, board lookup
│       ├── board.go       # Board / view board service operations
│       ├── lane.go        # Lane service operations
│       ├── card.go        # Card service operations + CardUpdate
│       ├── comment.go     # Comment service operations
│       ├── label.go       # Label service operations + LabelUpdate
│       ├── errors.go      # Service error types (NotFound, Conflict, …)
│       └── service_test.go
└── cmd/bankan/
    ├── main.go            # CLI (cobra) + newServeCmd() + newAISkillCmd()
    ├── server/
    │   ├── server.go      # HTTP server, routing, token middleware
    │   ├── handlers_api.go # REST JSON handlers
    │   ├── handlers_ui.go  # HTMX HTML fragment handlers + static serving
    │   └── server_test.go
    ├── skill/
    │   ├── skill.go       # //go:embed templates → skill.TemplateFS
    │   └── templates/
    │       ├── bankan.md.tmpl              # claude-code skill template
    │       └── bankan-agent-skill.md.tmpl # opencode / codex skill template
    └── ui/
        ├── layout.templ / layout_templ.go
        ├── board.templ  / board_templ.go
        ├── card.templ   / card_templ.go
        ├── types.go     # BoardPageData, CardDetailData, LaneWithCards
        ├── static.go    # //go:embed static → ui.StaticFS
        └── static/
            ├── style.css
            ├── app.js
            ├── htmx.min.js
            └── sortable.min.js
```

**Responsibilities:**

| File             | Responsibility                                                |
|------------------|---------------------------------------------------------------|
| `id.go`          | `GenerateID()`, `NewUniqueID()` — 5-char random alphanumeric |
| `frontmatter.go` | `Parse()`, `Serialize()` — generic YAML frontmatter codec    |
| `label.go`       | `Label` struct, `ValidateLabels`, `FindLabelByID/Name`       |
| `board.go`       | `Board` struct, `InitBoard`, `ReadBoard`, `WriteBoard`, `FindBoard`, label mutations |
| `lane.go`        | `Lane` struct, `ReadLanes`, `AddLane`, `RenameLane`, `RemoveLane`, `LaneByName` |
| `card.go`        | `Card` struct, `AddCard`, `ReadCard`, `WriteCard`, `FindCard`, `ListCards`, `MoveCard`, `ArchiveCard`, `RestoreCard`, `DeleteCard`, `ListArchivedCards`, `DuplicateCard` |
| `comment.go`     | `Comment` struct, `ReadComments`, `AddComment`, `SerializeComments` |
| `viewboard.go`   | `ViewBoard` struct, `InitViewBoard`, `ReadViewBoard`, `WriteViewBoard`, `ArchiveViewBoard`, `FindViewBoard`, `ParentBoard` |
| `viewcard.go`    | `ViewCardStub` struct, stub I/O, `ResolveViewCard`, `ListViewCards`, `ListViewCardStubs`, `FindViewCardStub`, `SyncViewBoard`, `AddViewCard`, `MoveViewCard`, `RemoveCardFromView` |
| `viewlane.go`    | `AddViewLane` (uniqueness across parent + view), `RemoveViewLane` |

All types are flat structs with YAML tags. No interfaces, no registries, no
dependency injection. Functions receive and return concrete values.

---

## View Boards

A view board is a **label-filtered, live subset view** of a parent board. It
does not copy card data — all card content lives in the parent board's files.

### Directory layout

```
<view-dir>/
├── view.md                           # View board metadata (sentinel file)
├── 01-backlog/                       # Cloned from parent at creation; view-only lanes added later
│   ├── 001-ab12c-fix-login.md        # Stub file (references parent card)
│   └── 002-xk9p2-add-oauth.md
├── 02-doing/
└── 03-sprint-icebox/                 # View-only lane (not in parent)
    └── 001-mn3rs-tech-debt.md
```

A directory is a view board if and only if it contains `view.md`. `IsViewBoard`
and `IsBoard` are mutually exclusive for the same directory.

### `view.md`

```yaml
---
name: Sprint 1 View
order: 2
parent: /absolute/path/to/parent/board
filter_label: ab12c
created_at: 2026-05-11T10:00:00Z
archived_at: null
---

Optional description.
```

| Field          | Notes                                                        |
|----------------|--------------------------------------------------------------|
| `name`         | Display name of the view board                               |
| `order`        | Tab bar position (1-based); `0`/absent means unordered, sorts last |
| `hidden`       | When `true`, excluded from tab bar and placed in overflow dropdown |
| `parent`       | Absolute path to the parent board directory                  |
| `filter_label` | Label ID from the parent board; **immutable** after creation |
| `archived_at`  | Set by `ArchiveViewBoard`; does not affect parent cards      |

### Stub files

Each card tracked in the view is represented by a stub file in a view lane
directory. Stub filenames follow the same `NNN-<id>-<slug>.md` pattern as
regular card files. The content is minimal:

```yaml
---
card_id: ab12c
---
```

The stub stores only the card ID. All card data (title, body, labels, comments)
is always read from the parent board's actual card file via `ResolveViewCard`.

### View board semantics

| Operation | Behaviour |
|---|---|
| `InitViewBoard` | Creates `view.md`, `_archive/`, and clones parent lanes |
| `SyncViewBoard` | Adds stubs for new parent cards with FilterLabel; removes orphaned stubs |
| `AddViewCard` | Creates card in parent (with FilterLabel), places stub in view lane |
| `MoveViewCard` to shared lane | Moves card file in parent + moves stub in view |
| `MoveViewCard` to view-only lane | Moves stub only; parent card file unchanged |
| `RemoveCardFromView` | Removes FilterLabel from parent card; removes stub; card preserved |
| `ArchiveViewBoard` | Sets `archived_at` in `view.md`; no effect on parent cards |
| `AddViewLane` | Adds lane to view only; name must be unique across parent + view |
| `RemoveViewLane` | Removes empty lane from view only; no effect on parent |

### Sync semantics (bidirectional)

`SyncViewBoard` reconciles the view with the current parent state:

1. Collect all parent cards with `FilterLabel` → **want set**.
2. Collect all card IDs from view stubs → **have set**.
3. For IDs in `want \ have`: create stub in the matching view lane (fallback to
   first view lane if the parent lane doesn't exist in the view).
4. For IDs in `have \ want`: remove orphaned stub (card no longer has the label).

---

## Card Lifecycle

```
         AddCard
            │
            ▼
    ┌───────────────┐◄──── DuplicateCard (new card, same lane, "[dup] " title prefix)
    │  Active card  │
    │  NNN-id-slug  │
    └───────────────┘
         │      │
    MoveCard   ArchiveCard
         │      │
         │      ▼
         │  ┌──────────────────┐
         │  │  Archived card   │  (_archive/, no order prefix)
         │  │  id-slug.md      │
         │  └──────────────────┘
         │      │
         │  RestoreCard ──────────────┐
         │                            ▼
         │                    ┌───────────────┐
         └──────────────────► │  Active card  │
                              └───────────────┘
                                      │
                                 DeleteCard
                                      │
                                      ▼
                               (removed from fs)
```

Key invariants:
- `archived_at` is set iff the card is in `_archive/`.
- `moved_at` / `moved_from` record only the **most recent** lane move; a full
  history would require the comments file or git log.
- `updated_at` is always ≥ `created_at`.
- Card IDs are unique within a board (enforced at creation by scanning all
  lanes and the archive).

---

## ID Generation

IDs are 5-character strings drawn from `[a-z0-9]` (36 characters), giving
`36^5 = 60,466,176` possible values. At creation, all existing card IDs across
all lanes and the archive are collected into a set, and a new ID is re-drawn
until it does not collide.

For comment IDs the same algorithm is applied within the existing IDs in the
target comments file.

Label IDs are generated with the same function but assigned by the CLI at
`label add` time; they are stored in `board.md` and validated for board-scoped
uniqueness.

---

## Lane and Card Ordering

**Boards in the tab bar** are ordered by the `order` field in `board.md` /
`view.md` (ascending). Boards with `order == 0` (field absent or explicitly
zero) are unordered and sort last, alphabetically by directory name among
themselves. Order is written by `service.Registry.ReorderBoards` (called from
`POST /api/v1/boards/reorder` or `bankan board reorder`). New boards created
via the service layer automatically get `order = max(existing) + 1`.

**Lanes** are ordered by the numeric prefix of their directory name. Adding a
lane appends `maxOrder + 1`. Renaming a lane preserves its prefix. Reordering
lanes via `POST /api/v1/boards/{id}/lanes/reorder` renames the directories with
redistributed `NN` prefixes.

**Cards within a lane** are ordered by the numeric prefix of their filename.
Adding or restoring a card appends `maxOrder + 1` within the target lane. Moving
a card also appends to the destination lane. The result is that moved cards
always appear at the bottom of the destination lane.

---

## CLI Design

The CLI binary `bankan` uses [cobra](https://github.com/spf13/cobra) for
subcommand routing. All `--board` flags accept both regular board directories
(containing `board.md`) and view board directories (containing `view.md`).
Commands dispatch automatically based on which type is detected.

```
bankan board init [<dir>] [--name <name>]
bankan board show [--board <dir>]
bankan board reorder --root <dir> <id1> <id2> ...   # set tab display order for all boards
bankan board hide --root <dir> <id>                 # hide board from tab bar
bankan board unhide --root <dir> <id>               # show hidden board in tab bar
bankan board view create <dir> --parent <path> --label <id> [--name <name>]
bankan board view sync [--board <view-dir>]
bankan board view show [--board <view-dir>]
bankan board view archive [--board <view-dir>]

bankan lane add <name> [--board <dir>]
bankan lane list [--board <dir>]
bankan lane rename <old> <new> [--board <dir>]
bankan lane remove <name> [--board <dir>]

bankan card add --lane <name> --title <text> [--body <text>] [--label <id>...] [--board <dir>]
bankan card list [--lane <name>] [--label <id>] [--archived] [--board <dir>]
bankan card show <id> [--board <dir>]
bankan card edit <id> [--title <t>] [--body <text>] [--add-label <id>] [--remove-label <id>] [--board <dir>]
bankan card move <id> --lane <name> [--board <dir>]
bankan card archive <id> [--board <dir>]
bankan card restore <id> --lane <name> [--board <dir>]  (not available in view boards)
bankan card delete <id> [--force] [--board <dir>]
bankan card duplicate <id> [--board <dir>]
bankan card reorder <card-id> <new-index> [--board <dir>]  (0-based position within its lane)
bankan card search <id> [--root <dir>] [--include-archived]  (searches all active boards under root)

bankan comment add <card-id> --text <text> [--author <name>] [--board <dir>]
bankan comment edit <comment-id> --card <card-id> --text <text> [--board <dir>]
bankan comment list <card-id> [--board <dir>]

bankan label add --name <name> --color <hex> [--board <dir>]  (not available in view boards)
bankan label list [--board <dir>]
bankan label edit <id> [--name <name>] [--color <hex>] [--board <dir>]
bankan label remove <id> [--board <dir>]

bankan ai-skill --type <type> <output-dir> [--with-bin-path]
```

`ai-skill` writes an AI agent skill file (`SKILL.md`) to `<output-dir>/bankan/`.
`--type` selects the target format: `claude-code`, `opencode`, or `codex`.
`claude-code` renders `cmd/bankan/skill/templates/bankan.md.tmpl`; `opencode`
and `codex` render `cmd/bankan/skill/templates/bankan-agent-skill.md.tmpl`.
Without `--with-bin-path` the skill references `bankan` by name; with the flag
it substitutes the absolute path of the running binary, useful when the binary
is not on the agent's `PATH`.

`card search` uses `--root` (like `board reorder`, `board hide`, `board unhide`)
instead of `--board`, because it operates across all boards under a container
directory rather than on a single board. Archived view boards are skipped by
default; `--include-archived` opts in to searching them.

**Board resolution order** for `--board`:

1. Explicit `--board <dir>` flag.
2. Check if the current working directory is a view board (`view.md` present).
3. Walk up from the current working directory looking for `board.md`
   (`FindBoard`), then fall back to walking up looking for `view.md`
   (`FindViewBoard`).

**View board command behaviour differences:**

| Command | Regular board | View board |
|---|---|---|
| `card add` | Creates in board | Creates in parent with filter label auto-applied |
| `card archive` | Archives to `_archive/` | Removes filter label from parent card; card stays |
| `card delete` | Permanently deletes | Removes filter label from parent card; card stays |
| `card restore` | Restores from `_archive/` | Error: not available |
| `card duplicate` | Duplicates card in same lane with `[dup] ` title prefix | Error: not available |
| `card move` to shared lane | Moves card file | Moves card file in parent + moves stub |
| `card move` to view-only lane | N/A | Moves stub only; parent card unchanged |
| `label add` | Adds label to board | Error: not available |
| `label remove` of filter label | Allowed | Blocked: filter label is immutable |
| `label remove` of other label | Allowed | Forwarded to parent board |
| `lane add` | Adds lane to board | Adds view-only lane (unique across parent + view) |
| `lane remove` | Removes empty lane | Removes empty lane from view only |

**Author resolution** for `comment add`:

1. `--author` flag.
2. `git config user.name` output.
3. `$USER` environment variable.
4. Literal `"unknown"`.

**`card delete`** requires `--force` to prevent accidental permanent deletion.

---

## Service Layer (`internal/service`)

`internal/service` is a thin orchestration layer consumed by both the CLI
(`cmd/bankan/main.go`) and the HTTP server (`cmd/bankan/server`). It wraps the
root `bankan` package calls with:

- **`Registry`** — holds a map of board ID → `*Board` / `*ViewBoard` with a
  per-board `sync.RWMutex` for concurrent-safe access. Read operations acquire
  a read lock; mutations acquire a write lock.
- **`NewSingleRegistry(dir)`** — loads a single board or view board from a
  known directory; used by the CLI.
- **`NewRegistry(dirs []string, rootDir string)`** — loads multiple boards
  from an explicit list and optionally scans a root directory for boards; used
  by the HTTP server.
- **Error types** — `NotFoundError`, `ConflictError`, `ValidationError`,
  `ForbiddenError` allow callers to map service errors to appropriate HTTP
  status codes or CLI exit messages.

The CLI always uses `NewSingleRegistry`; the server uses `NewRegistry` with
one or more board directories passed via `--board` flags or positional
arguments.

---

## REST API (`cmd/bankan/server`)

### Server

`server.Server` wraps a `*service.Registry` and a `net/http.ServeMux` wired
with Go 1.22 method+path routing patterns (`"POST /api/v1/boards/{id}/cards"`
etc.).

**Security**: all mutating requests (`POST`, `PATCH`, `PUT`, `DELETE`) require
an `X-Bankan-Token` header matching the token printed at server startup.
`GET` requests do not require the token. Token requirement can be disabled
with `--no-token`.

**Bind address**: defaults to `127.0.0.1` (localhost only).

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/boards` | List all registered boards (in display order) |
| POST | `/api/v1/boards` | Create a new board (requires `rootDir`) |
| POST | `/api/v1/boards/reorder` | Set tab display order; body `{"ids":[...]}` with all board IDs |
| GET | `/api/v1/boards/{id}` | Get board details + labels |
| POST | `/api/v1/boards/{id}/hide` | Hide board from tab bar |
| POST | `/api/v1/boards/{id}/show` | Restore hidden board to tab bar |
| GET | `/api/v1/boards/{id}/lanes` | List lanes |
| POST | `/api/v1/boards/{id}/lanes` | Add lane |
| PATCH | `/api/v1/boards/{id}/lanes/{name}` | Rename lane |
| DELETE | `/api/v1/boards/{id}/lanes/{name}` | Remove empty lane |
| POST | `/api/v1/boards/{id}/lanes/reorder` | Reorder lanes; body `{"names":[...]}` |
| GET | `/api/v1/boards/{id}/cards` | List cards (`?archived=true` for archive) |
| POST | `/api/v1/boards/{id}/cards` | Add card |
| GET | `/api/v1/boards/{id}/cards/{cardId}` | Get card |
| PATCH | `/api/v1/boards/{id}/cards/{cardId}` | Update card |
| DELETE | `/api/v1/boards/{id}/cards/{cardId}` | Delete card (`?force=true` required) |
| POST | `/api/v1/boards/{id}/cards/{cardId}/move` | Move card to lane |
| POST | `/api/v1/boards/{id}/cards/{cardId}/reorder` | Move card to position within its lane; body `{"new_index": N}` (0-based) |
| POST | `/api/v1/boards/{id}/cards/{cardId}/archive` | Archive card |
| POST | `/api/v1/boards/{id}/cards/{cardId}/restore` | Restore card to lane |
| POST | `/api/v1/boards/{id}/cards/{cardId}/duplicate` | Duplicate card in same lane; returns 201 with new card JSON |
| GET | `/api/v1/boards/{id}/cards/{cardId}/comments` | List comments |
| POST | `/api/v1/boards/{id}/cards/{cardId}/comments` | Add comment |
| PATCH | `/api/v1/boards/{id}/cards/{cardId}/comments/{commentId}` | Edit comment body |
| GET | `/api/v1/boards/{id}/labels` | List labels |
| POST | `/api/v1/boards/{id}/labels` | Add label |
| PATCH | `/api/v1/boards/{id}/labels/{labelId}` | Update label |
| DELETE | `/api/v1/boards/{id}/labels/{labelId}` | Remove label |
| POST | `/api/v1/boards/{id}/sync` | Sync view board with parent |
| POST | `/api/v1/boards/{id}/archive` | Archive a view board |
| POST | `/api/v1/view-boards` | Create a new view board; body `{"name","parent_id","filter_label_id"}` |

Board ID in the URL is the **directory basename** of the board.

### UI (HTMX)

`handlers_ui.go` serves a dark kanban board UI using HTMX and SortableJS.
All static assets (CSS, JS) are embedded via `//go:embed` in
`cmd/bankan/ui/static.go` and served under `/static/`.

The UI uses `cmd/bankan/ui/*.templ` templates (compiled with `templ generate`
to `*_templ.go`). After any `.templ` file change, run:

```bash
templ generate ./cmd/bankan/ui/
```

The UI supports: board listing, lane display, card display, card detail modal,
card creation, card editing, card move, drag-and-drop card reordering between
lanes (SortableJS), drag-and-drop board tab reordering (SortableJS, persisted
via `POST /api/v1/boards/reorder`), and label display.

### `bankan serve` subcommand

```
bankan serve [<dir>...] [--board <dir>]... [--port <n>] [--bind <addr>]
             [--token <tok>] [--no-token] [--root <dir>]
```

- One or more board directories may be provided as positional args or via
  repeated `--board` flags.
- `--root <dir>` scans a directory for boards and registers them all.
- `--port` defaults to `8080`.
- `--bind` defaults to `127.0.0.1`.
- `--token` sets a fixed token; otherwise one is generated and printed at
  startup.
- `--no-token` disables token enforcement entirely.

---

## Testing Strategy

**Unit tests** (`*_test.go` in package `bankan`) cover each component in
isolation using `t.TempDir()` for real filesystem operations. No mocking.

**Integration tests** (`lifecycle_integration_test.go` in package `bankan_test`)
exercise full multi-step workflows end-to-end:

- Board init, read, multiple boards in the same parent directory.
- Lane CRUD (add, list, rename, remove).
- Card CRUD (add, read, edit, list, delete).
- Card ordering within a lane.
- Move card across lanes (includes comments migration).
- Archive → restore round-trip (includes comments migration).
- Label management (add, update, remove, uniqueness enforcement, label on card).
- `FindBoard` walking up from a deeply nested subdirectory.
- `DeleteCard` with a comments file present.
- View board: create, sync, add card via view, move to shared lane, move to view-only lane.
- View board: remove card from view (filter label removed, card preserved in parent).
- View board: archive view board (cards unaffected in parent).
- View board: bidirectional sync (new cards added, orphaned stubs removed).
- View board: `FindViewBoard` walk-up from nested subdirectory.
- View board: view and parent board as siblings in the same directory tree.

All tests use `t.TempDir()` so cleanup is automatic and tests are parallel-safe.
