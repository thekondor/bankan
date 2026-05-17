# AGENTS.md — Context and Extension Guide for LLM Agents

This file is the primary entry point for any AI agent working on this codebase.
Read it before making any changes, then follow the links below for the topic you need.

---

## Project Purpose

`bankan` is a **local-first kanban board manager**. All state is stored in plain
markdown files with YAML frontmatter — no database, no daemon, no network. A
board is a directory; copying or moving it is a complete backup.

The primary use-case is keeping boards inside a software project and tracking
them with git.

The module is `github.com/thekondor/bankan`, written in Go 1.26.

---

## Instructions

- Always ask questions in case of concerns, do not make assumptions.
- Always update or add new unit and integration tests for new logic.
- Always keep API for CLI and REST endpoint equal.
- **When CLI command or REST endpoint behavior changes, update `cmd/bankan/server/cli_vs_rest_test.go`
  and `lifecycle_integration_test.go`** — these files must always reflect current behavior.
  See [How to write tests](agents/testing.md) for the exact rules.
- **Always update the relevant file(s) in `agents/` when you add, remove, or change
  anything described there** (file formats, invariants, conventions, how-to steps,
  limitations, layout). If you add a new topic that has no home yet, create a new
  file under `agents/` and add it to the Table of Contents below.

---

## Table of Contents

- [Repository layout and package boundaries](agents/repository-layout.md) — directory tree, which packages own what, and how the library / service / CLI layers relate.
- [File format specification](agents/file-format.md) — board.md, lane directories, card filenames, comments files, view board structure and sync semantics.
- [Key invariants](agents/invariants.md) — rules that must never be broken: ID uniqueness, slug immutability, comment file co-location, display-order source of truth, and more.
- [Go style conventions and dependencies](agents/conventions.md) — coding style rules, error-wrapping pattern, test guidelines, and the approved dependency list.
- [How to extend the codebase](agents/howto-extend.md) — step-by-step guides for adding card fields, lifecycle operations, CLI commands, REST endpoints, UI templates, view board operations, and lane/board operations.
- [How to write tests](agents/testing.md) — unit test helpers, integration test requirements, and how to run the test suite.
- [Known limitations and out-of-scope features](agents/limitations.md) — intentional limitations and a list of features that must not be added.
