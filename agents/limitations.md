# Known Limitations and Scope

## Known Limitations (intentional, not bugs)

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

## Things That Are Explicitly Out of Scope (do not add)

- Network sync, conflict resolution, or cloud storage.
- Authentication or multi-user access control (the HTTP server uses a
  single static token; there is no per-user identity).
- Binary attachments — the format is text-only by design.
- A database or index file — all queries scan the filesystem.
- Sub-packages inside the root `bankan` package — keep the library flat.
