---
phase: 01-connect-and-read
plan: 05
subsystem: tools
tags: [go, mcp, gitlab, repository-tree, pagination]
requires:
  - phase: 01-connect-and-read (plan 04)
    provides: OutputBudget, Budget, composeList, PageFooter, ClampPaging, withSubject, list_projects/get_project pattern
provides:
  - resolveRef / refLabel / errEmptyRepo (default-branch resolution reused by get_file_contents)
  - list_repository_tree tool (READ-03)
affects: [01-06 get_file_contents]
tech-stack:
  added: []
  patterns: ["explicit ref costs no request; empty ref -> GetProject default_branch, never symbolic HEAD"]
key-files:
  created:
    - internal/tools/ref.go
    - internal/tools/ref_test.go
    - internal/tools/tree.go
    - internal/tools/tree_test.go
  modified:
    - internal/tools/register.go
key-decisions:
  - "No default-branch cache in Phase 1: an empty ref costs one GetProject per call (RESEARCH Open Q5)"
  - "Empty listing returns header plus '(пусто)' without a pagination footer"
requirements-completed: [READ-03, FND-04, FND-06]
metrics:
  completed: 2026-09-24
---

# Phase 1 Plan 05: list_repository_tree Summary

**`list_repository_tree` browses a repository at any path/ref with explicit default-branch resolution, pagination and a 15 000-rune output budget.**

## Accomplishments
- `resolveRef` returns an explicit ref untouched (no request) or the project's named default branch; empty repositories give a clear "репозиторий пуст" error instead of a 404; `HEAD` is never sent.
- `list_repository_tree` normalises project and path (rejects `..`), sends `ref` always, `path`/`recursive` only when set, labels the default branch in the header, renders `dir  x/`, `file x`, `sub  x` lines with full paths, and composes truncation and page footers.
- Wire test locks the exact RequestURI for a path with spaces, Cyrillic, `+`, `#`, `%` and a ref containing `/`.
- Tool registered fourth (whoami, list_projects, get_project, list_repository_tree); `TestToolSchemas` now covers 4 tools.

## Task Commits
1. Task 1: default-branch resolution - `cd793ff`
2. Task 2: list_repository_tree tool - `9b8b7be`

## Verification (actually run)
- `go test ./internal/tools/ -run 'ResolveRef|RefLabel' -count=1 -v`: 6 tests PASS.
- `go vet ./...`: clean.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- `grep -n '"HEAD"' internal/tools/ref.go`: no output.
- Not verified: behaviour against real gitlab.com (only the fake GitLab server was used).

## Deviations from Plan
None - plan executed as written. (`gofmt -l` lists every file in the repo because of CRLF working-copy line endings; pre-existing and out of scope, `go vet` is clean.)

## Known Stubs
None.

## Self-Check: PASSED
Files ref.go, ref_test.go, tree.go, tree_test.go, register.go exist; commits cd793ff and 9b8b7be exist.
