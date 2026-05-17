# Key Invariants — Never Break These

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
