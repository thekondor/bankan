# How to Write Tests

## Unit test (testing a single function)

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

## Integration test (package `bankan_test`)

Add to `lifecycle_integration_test.go`. Import as:
```go
import bankan "github.com/thekondor/bankan"
```

Integration tests must:
- Exercise a multi-step workflow (at least 3 operations).
- Verify filesystem state (file existence, file contents via re-read) not just
  return values.
- Name the test `TestLifecycle_<WorkflowName>`.

## CLI vs REST equivalence tests — keep always in sync

`cmd/bankan/server/cli_vs_rest_test.go` contains end-to-end equivalence tests
that run the same sequence of operations through two independent code paths —
real CLI subprocesses and a live `bankan serve` process — and assert identical
filesystem state. These tests are the authoritative proof that CLI and REST
behave identically.

**Whenever you change the observable behavior of any CLI command or any REST
endpoint** (new operation, changed response, changed state transition), you
**must** update this file to cover the new behavior. Specifically:

- If a new CLI command or REST endpoint is added, add a parallel
  `runXxxViaRealCLI` / `runXxxViaRealREST` pair and a
  `TestLifecycle_CLIvsREST_Xxx` test that compares their snapshots.
- If an existing operation's behavior changes (e.g. new field in response,
  different state written to disk), update the relevant snapshot type and
  both runner functions so they capture and assert the new behavior.
- Never remove an equivalence test without replacing it — a missing test is a
  silent gap in the CLI/REST contract.

Similarly, `lifecycle_integration_test.go` (package `bankan_test`) tests the
multi-step library workflows end-to-end. **Whenever a card, lane, board, view
board, label, or comment lifecycle operation changes**, update or add a
`TestLifecycle_*` test in that file to reflect the current behavior.

## Running tests

```bash
go test ./...               # all tests
go test -run TestFoo ./...  # specific test
go test -v -count=1 ./...   # verbose, no cache
```
