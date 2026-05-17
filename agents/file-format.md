# File Format

This is the single most important thing to understand about the project.
All state lives in plain markdown files with YAML frontmatter.

---

## Board directory

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

## Board `board.md`

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

## Lane directory naming

```
NN-<slug>
```

- `NN` is a two-digit zero-padded integer starting at `01`.
- `slug` is the display name lowercased, spaces → hyphens, special chars dropped.
- `_archive` is reserved and never treated as a lane.
- `parseLaneDir(base)` is the canonical parser. It returns `(order int, name string, ok bool)`.
- `ReadLanes` ignores any directory whose name does not match `laneNameRe`.

## Card file naming (in a lane)

```
NNN-<id>-<slug>.md
```

- `NNN` is a three-digit zero-padded order prefix (1–999).
- `id` is exactly 5 lowercase alphanumeric characters (`[a-z0-9]{5}`).
- `slug` is derived from the title at creation, **frozen forever** (slug never
  changes even if the title is later edited).
- `slug` must not contain `.` — the regex `cardFileRe` uses `[^.]+` for the
  slug group specifically to exclude `.comments.md` files from matching as cards.

## Card file naming (in `_archive/`)

```
<id>-<slug>.md
```

No order prefix. Archive files are matched by `archCardRe`:
`^([a-z0-9]{5})-([^.]+)\.md$`

## Comments file naming

The comments file shares the base name of its card but with `.comments.md`:

```
001-ab12c-fix-login.md          → 001-ab12c-fix-login.comments.md
ab12c-fix-login.md (archive)   → ab12c-fix-login.comments.md
```

`commentFilename(cardBase string) string` handles this. Comments files are
**always co-located** with their card and are moved/renamed atomically with it.

## Comments file content

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

## View board directory

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

## View board stub file naming

Stubs in view lane directories use the **same filename pattern** as regular cards:
`NNN-<id>-<slug>.md`. The slug is derived from the card title at stub creation
time. The content is minimal:

```yaml
---
card_id: ab12c
---
```

## View board sync semantics

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

## View board `view.md`

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
