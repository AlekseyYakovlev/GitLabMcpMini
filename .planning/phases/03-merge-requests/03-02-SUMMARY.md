---
phase: 03-merge-requests
plan: 02
subsystem: api
tags: [gitlab, merge-requests, diff, notes, mcp, go, client-go]

requires:
  - phase: 03-merge-requests
    provides: mrSubject, checkIID, MR test fixtures (mrJSON, mr5Path) from plan 01
  - phase: 02-history-and-write
    provides: renderDiffFiles, diffBodySlack, composeList, PageFooter, Budget
provides:
  - get_merge_request_diffs tool (typed /diffs call rendered like get_commit)
  - list_merge_request_notes tool ([system] marker, per-note cap, footer)
  - 16-tool golden snapshot, stdio e2e list and Python mcp 1.30.0 smoke coverage
affects: [03-03, 03-04, 03-05]

tech-stack:
  added: []
  patterns:
    - "MR diff adapter mrDiffToDiffFile feeds the shared renderDiffFiles"
    - "Notes rendered by noteLines; create_merge_request_note is to be added to mr_notes.go"

key-files:
  created:
    - internal/tools/mr_diff.go
    - internal/tools/mr_diff_test.go
    - internal/tools/mr_notes.go
    - internal/tools/mr_notes_test.go
  modified:
    - internal/tools/register.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "The MR diff uses the typed ListMergeRequestDiffs; it already carries collapsed and too_large, so no raw request is needed"
  - "System notes are flattened with oneLine (first line only, 120 runes), which drops the bullet list GitLab appends to 'added N commits'"
  - "Author of a note without a username shows as @-"

patterns-established:
  - "Empty page text depends on changes_count: empty means diff not ready, otherwise no changes on this page"

requirements-completed: [MR-03, MR-04]

duration: 20min
completed: 2026-09-24
---

# Phase 3 Plan 02: MR diff and notes Summary

**get_merge_request_diffs (typed /diffs, same renderer and budget as get_commit, explicit reason for every empty patch, overflow line) and list_merge_request_notes (newest first, [system] one-liners, user bodies cut at 1000 runes), pinned at 16 tools through the real binary and Python mcp 1.30.0**

## Flag for the author: the overflow signal is a surrogate

GitLab's `/merge_requests/:iid/diffs` response has no `overflow` field (RESEARCH F1). The line "часть файлов не вернулась: в MR больше 1000 файлов (changes_count=1000+) ..." is derived from `changes_count` ending in "+", which GitLab caps at "1000+". It is not a direct server flag. The tests never put an `overflow` key in the fake `/diffs` JSON.

## Accomplishments

- get_merge_request_diffs: GET MR then GET `/diffs?page&per_page` (in that order). Header `!iid title`, `ветки: src→dst`, `файлов на странице: N`. Body through `renderDiffFiles` with budget `OutputBudget - header - diffBodySlack`; refs for hints are `sha`/`diff_refs.base_sha` with branch fallbacks for fresh MRs.
- Empty page: "diff ещё готовится, повторите через несколько секунд" when `changes_count` is empty, otherwise "изменений в файлах нет (страница за пределами списка или MR без diff)". Files with empty patches show too_large, collapsed or rename reasons and never read as "no changes".
- list_merge_request_notes: one request, no order_by/sort (GitLab default is created_at desc). System note is `[system] <first line, 120 runes>`; user note is `#id @user date` plus body cut at 1000 runes with `[заметка обрезана]`; null created_at shows `-`; empty list shows "заметок нет".
- Both tools ReadOnlyHint; golden, stdio e2e and scripts/smoke.py expect 16 tools; smoke calls both.

## Task Commits

1. **Task 1: get_merge_request_diffs** - `d36bd5f` (feat)
2. **Task 2: list_merge_request_notes** - `3efd2b5` (feat)
3. **Task 3: 16 tools through the real binary and Python client** - `b851ace` (test)

## Verification (observed)

- `go vet ./...` clean.
- `go test ./... -count=1`: all packages ok (internal/tools, e2e, glclient, config, logging).
- `go test ./e2e -v -run TestPythonSmoke`: PASS (not skipped), real binary + Python mcp 1.30.0 client calling both new tools.
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: succeeded.
- gofmt clean on all changed Go files when checked through `tr -d '\r'` (working tree is CRLF, see plan 01 note).
- acceptance greps: `"overflow"` in mr_diff_test.go = 0; no pointer fields in mr_diff.go; no OrderBy/Sort in mr_notes.go code.
- Not verified: live gitlab.com behaviour (planned for Phase 4, QA-03).

## Deviations from Plan

None in behavior. Process note: TDD tasks were written implementation-first with tests immediately after (all tests passed on first run), so there is no separate failing-test commit for tasks 1 and 2.

## Assumption Drift (advisory)

- **Found during:** Task 2
- **Planned:** system note body "flattened" (behavior list) with the plan's helper set.
- **Actual:** implemented with the existing `oneLine` helper, which keeps only the first line; multi-line system bodies lose their bullet lines.
- **Why:** one short line per system note is the stated goal (D-07); the first line carries the event.

## Known Stubs

None.

## Threat Flags

None. T-03-06..T-03-09 mitigations are in place: framed headings and `[system]` marker, per-file and per-note caps within OutputBudget, explicit empty-patch reasons, error wording via `withSubject`.

## Self-Check: PASSED

- internal/tools/mr_diff.go, mr_diff_test.go, mr_notes.go, mr_notes_test.go: present
- commits d36bd5f, 3efd2b5, b851ace: present in git log
