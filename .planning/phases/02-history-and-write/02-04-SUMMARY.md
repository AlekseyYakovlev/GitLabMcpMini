---
phase: 02-history-and-write
plan: 04
subsystem: api
tags: [go, mcp, gitlab, commits, write, json-schema]

requires:
  - phase: 02-history-and-write (plan 01)
    provides: withWrite, writeRules, opCommit, create_branch
  - phase: 02-history-and-write (plan 02)
    provides: diffFile, fetchCommitDiff, countPatchLines, shortSHA
provides:
  - commit_files tool (one POST /repository/commits with create/update/delete/move actions)
  - hand-written nested actions[] inputSchema (commitFilesSchema)
  - per-file +/- from a follow-up diff read
  - commit write error wording (protected branch, missing branch, file exists / missing)
  - golden tools/list snapshot (testdata/tools_list.golden.json)
affects: [03 merge requests, create_or_update_file]

tech-stack:
  added: []
  patterns:
    - "Explicit map[string]any InputSchema when a tool needs nested objects; no jsonschema-go import"
    - "Golden snapshot of tools/list with a -update flag"
    - "Write followed by a read-only follow-up whose failure never turns the write into an error"

key-files:
  created:
    - internal/tools/schema.go
    - internal/tools/write.go
    - internal/tools/write_test.go
    - internal/tools/testdata/tools_list.golden.json
  modified:
    - internal/tools/register.go
    - internal/tools/schema_test.go
    - internal/tools/errors.go
    - internal/tools/errors_test.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "Per-file +/- follows locked D-05 (follow-up GET of the new commit's diff) instead of RESEARCH A1 (commit totals only); RESEARCH Open Question 2 treated as resolved this way"
  - "The stats key for a file is its new path, or the old path for a deleted file; a rename-only or empty-new-file entry counts as known +0/-0"
  - "commit_files annotated DestructiveHint=true because it can delete and overwrite files"
  - "A create/update action without content sends an empty file (the schema cannot tell an omitted content from an empty one because content is omitempty)"

patterns-established:
  - "toFileAction / validateCommitInput / commitCore are reusable by a later single-file write tool"

requirements-completed: [WRT-02]

duration: 35min
completed: 2026-09-24
---

# Phase 2 Plan 04: commit_files Summary

**commit_files makes one commit with several create/update/delete/move actions through a hand-written nested actions[] schema, sends the POST exactly once, and reports exact per-file +/- from a follow-up diff read.**

## Performance

- **Tasks:** 3/3
- **Files:** 4 created, 7 modified

## Accomplishments

- `commit_files` (WRT-02): input guards run before any request (empty or over 50 actions, over 1 MiB of content, blank message or branch, unknown action, `..` in a path, move without `previous_path`, NUL or non-UTF-8 content, `previous_path` on a non-move, content on delete). One POST, never retried. The body carries only branch, commit_message and actions (no force, start_branch, encoding, stats).
- The result shows the short SHA, branch, commit totals, a pluralised file count (1 файл / 2 файла / 5 файлов), one line per action with per-file +/- and the web_url. A failed follow-up diff GET leaves the commit successful, with `+?/−?` per file and a pointer to `get_commit`.
- `tools/list` for `commit_files` has `actions` as `{type: array, items: {type: object}}` with `additionalProperties: false` at both levels and no null/$ref/anyOf. It is pinned by `TestToolsListGolden` and accepted by the real Python mcp 1.30.0 client, which then calls the tool with nested actions.
- Commit write errors: protected branch, missing branch, file already exists / does not exist have Russian texts with the real HTTP status prefix; 5xx and timeouts say the outcome is unknown.

## Task Commits

1. Task 1: commit_files end to end, schema, guards, one POST, per-file result, golden snapshot - `889dde6`
2. Task 2: commit write errors - `fb1f7c5`
3. Task 3: real-client proof (Go binary + Python mcp 1.30.0) - `b75d9c8`

## Verification (observed)

- `go vet ./...`: clean.
- `go test ./... -count=1`: all packages `ok`, including `internal/tools` (TestToolSchemas, TestToolsListGolden, TestPluralFiles, TestPerFileStats, guard, failure and body tests) and `e2e`.
- `go test ./e2e -run 'TestPythonSmoke|TestStdio' -v`: `TestPythonSmoke` PASS (1.71 s, not skipped, real `mcp==1.30.0` via `uv`), which parsed the nested schema and called `commit_files` with two nested actions.
- Golden file: 0 occurrences of `"null"`; `commit_files` entry has `actions.type` = `array` and `items` present.
- Acceptance greps: no `Force|StartBranch|Encoding|ExecuteFilemode` in non-comment lines of write.go; no `jsonschema-go` import in schema.go and no direct `go.mod` requirement; no status digits inside `text:` of writeRules.
- Not verified: calling commit_files against real gitlab.com (needs a live PAT with `api` scope; the smoke script runs the write tools only in hermetic mode). The exact GitLab error phrasings (RESEARCH A3) are taken from the research, not observed live.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Test constant names collided with existing ones**
- **Found during:** Task 1
- **Issue:** `commitSHA` (and similar names) were already declared in the `tools` test package.
- **Fix:** Renamed my test constants (`newCommitSHA`, `newCommitDiff`, `commitPostPath`, `commitPostReq`, `newCommitJSON`).
- **Files modified:** internal/tools/write_test.go
- **Commit:** 889dde6

**2. [Rule 1 - Bug] Trailing space dropped by an edit broke gofmt**
- **Found during:** Task 3
- **Issue:** `testToken) ||strings.Contains` (missing space) in e2e/python_smoke_test.go.
- **Fix:** Restored the space; gofmt is clean again.
- **Commit:** b75d9c8

### Plan-level notes

- The plan's gofmt check `test -z "$(gofmt -l ...)"` lists every CRLF-checked-out file; I verified formatting of the touched files on LF-normalised content instead (all clean).
- The plan recorded the D-05 vs RESEARCH A1 deviation up front (per-file stats through a follow-up GET); implemented as planned.

## Assumption Drift (advisory)

None material.

## Known Stubs

None.

## Threat Flags

None. The new write surface (POST /repository/commits) is covered by the plan's threat model (T-02-14 to T-02-20).

## Self-Check: PASSED

- FOUND: internal/tools/schema.go, internal/tools/write.go, internal/tools/write_test.go, internal/tools/testdata/tools_list.golden.json
- FOUND commits: 889dde6, fb1f7c5, b75d9c8
