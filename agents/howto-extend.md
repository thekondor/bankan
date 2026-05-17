# How to Extend the Codebase

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
