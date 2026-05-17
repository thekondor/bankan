# Go Style Conventions

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

# Dependency Summary

| Package | Purpose |
|---|---|
| `gopkg.in/yaml.v3` | YAML frontmatter marshal/unmarshal |
| `github.com/spf13/cobra` | CLI subcommand routing |
| `github.com/stretchr/testify` | Test assertions (`assert`, `require`) |
| `github.com/a-h/templ` | Type-safe HTML templates (UI layer only) |

Do not add new dependencies without a strong reason. The library (`package bankan`)
must not import `cobra`, `templ`, or `net/http` — those dependencies belong
only in `cmd/bankan` and `internal/service`.
