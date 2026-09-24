---
phase: 02-history-and-write
plan: 05
subsystem: api
tags: [go, mcp, gitlab, repository-files, write, optimistic-locking]

requires:
  - phase: 02-history-and-write (plan 04)
    provides: commitCore, validateCommitInput, fileAction, golden tools/list snapshot
  - phase: 02-history-and-write (plan 01)
    provides: withWrite, writeRules, opCommit
provides:
  - create_or_update_file tool (GET on the target branch decides create vs update, one shared commit POST)
  - decodeFileContent shared by get_file_contents and create_or_update_file
  - stale-write conflict wording ("файл изменился с момента чтения") with the real HTTP status
  - 12-tool golden snapshot, stdio e2e and Python mcp 1.30.0 smoke coverage
affects: [03 merge requests, 04 quality gate]

tech-stack:
  added: []
  patterns:
    - "Single write path: single-file tool builds one fileAction and reuses validateCommitInput + commitCore"
    - "Optimistic lock: update carries last_commit_id read on the same branch"

key-files:
  created:
    - internal/tools/upsert_test.go
  modified:
    - internal/tools/file.go
    - internal/tools/write.go
    - internal/tools/register.go
    - internal/tools/errors.go
    - internal/tools/errors_test.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "Content is always sent as given, even when identical to the current file (D-08); RESEARCH A4 (skip the POST) not adopted"
  - "Binary existing file: CRLF check skipped, text content written as given; RESEARCH Pattern 6 (refuse binary overwrite) not adopted"
  - "GET failures other than 404 are read errors (withSubject), never wrapped as write failures, and lead to zero POSTs"
  - "create_or_update_file annotated DestructiveHint=true (overwrites content)"

patterns-established:
  - "Conflict rule is op-agnostic in writeRules and carries no status digits, so 400 and 409 both read correctly"

requirements-completed: [WRT-03]

duration: 30min
completed: 2026-09-24
---

# Phase 2 Plan 05: create_or_update_file Summary

**create_or_update_file reads the file on the target branch to choose create or update, sends last_commit_id with updates so concurrent edits are rejected, and commits through the same single POST path as commit_files without ever retrying.**

## Performance

- **Tasks:** 3/3
- **Files modified:** 10 (1 created)

## Accomplishments
- New tool `create_or_update_file` (WRT-03): GET `.../repository/files/<path>?ref=<branch>`; 404 becomes action create, 200 becomes update with `last_commit_id`; exactly one `POST /repository/commits` through `commitCore`. Result: `created|updated <path>: коммит <short> в <branch>`, optional CRLF warning line, web_url.
- `decodeFileContent` extracted from `getFileContents` (behaviour unchanged, Phase 1 tests pass).
- CRLF to LF change reported as a warning, content is never normalised.
- Stale-write conflict rule in `writeRules` (`changed since you started editing`), worded without status digits so 400 and 409 both show their real code.
- Golden tools/list regenerated in Task 1 (diff is 42 added lines, only the new tool); e2e and `smoke.py` now cover 12 tools, hermetic smoke calls `create_or_update_file` and expects `updated README.md`.

## Task Commits

1. **Task 1: create_or_update_file + golden** - `21bece3` (feat)
2. **Task 2: conflict wording and write error states** - `3941256` (feat)
3. **Task 3: 12-tool e2e and Python smoke** - `f9f2162` (test)

## Verification (observed)

- `go test ./internal/tools/ -count=1` ok (includes TestToolsListGolden with regenerated snapshot, all upsert/conflict cases, Phase 1 get_file_contents tests).
- `go vet ./...` clean; `go test ./... -count=1` ok for e2e, config, glclient, logging, tools.
- `TestPythonSmoke` PASS (2.83 s, not skipped): real binary driven by Python `mcp==1.30.0` via `uv`, output contains `updated README.md`.
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp` succeeded (11.25 MB; exe is git-ignored).
- Acceptance greps: `resolveRef(` in write.go (non-comment) count 0; golden has 12 `"name"` entries and `create_or_update_file`; no status digits in `writeRules` texts.
- gofmt: the worktree is CRLF, so `gofmt -l` lists every file; verified touched files with CR stripped (`tr -d '\r' | gofmt -l`), no findings.
- NOT verified: real gitlab.com write behaviour (out of scope, Phase 4 QA-03).

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written. TDD note: tests and implementation for Task 1 and 2 were committed together per task (no separate RED commit); tests were run against the implementation and pass.

Recorded RESEARCH deviations follow the plan's own objective section (A4 skip-identical-POST and Pattern 6 binary refusal not adopted, per locked D-08).

## Known Stubs

None.

## Threat Flags

None - no new surface beyond the plan's threat model (T-02-21..T-02-25 mitigated: last_commit_id, path normalisation, size guards before any request, single non-retried POST, content sent byte-for-byte).

## Self-Check: PASSED
