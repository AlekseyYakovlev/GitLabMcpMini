---
phase: 03-merge-requests
plan: 01
subsystem: api
tags: [gitlab, merge-requests, mcp, go, client-go]

requires:
  - phase: 02-history-and-write
    provides: Deps/safe handler pattern, Budget/composeList/PageFooter, fake GitLab and in-memory MCP test harness
provides:
  - list_merge_requests tool (default state=opened sent on the wire, optional filters)
  - get_merge_request tool (short header, one request, no polling)
  - mergeStatusAdvice table with statusAdvice and stateAdvice, shared with the future merge pre-check
  - 14-tool golden snapshot, stdio e2e list and Python mcp 1.30.0 smoke coverage
affects: [03-02, 03-03, 03-04, 03-05]

tech-stack:
  added: []
  patterns:
    - "MR code in mr.go plus mr_<concern>.go files, tests <file>_test.go"
    - "Pure lookup table (mergeStatusAdvice) unit-tested without HTTP"

key-files:
  created:
    - internal/tools/mr.go
    - internal/tools/mr_status.go
    - internal/tools/mr_test.go
    - internal/tools/mr_status_test.go
  modified:
    - internal/tools/register.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "state is always sent to GitLab (default opened) because GitLab itself defaults the list to all"
  - "has_conflicts is annotated '(ещё не проверено)' while the status is checking/unchecked"
  - "Unknown detailed_merge_status is printed raw with a generic hint; a non-open state is explained before the status"

patterns-established:
  - "mrSubject 404 wording 'MR (iid) или проект' for every iid-based call"
  - "checkIID guard runs before any request"

requirements-completed: [MR-01, MR-02]

duration: 25min
completed: 2026-09-24
---

# Phase 3 Plan 01: list and get Merge Requests Summary

**list_merge_requests (opened by default, filters) and get_merge_request (one request, explained detailed_merge_status) with a shared status-advice table, pinned at 14 tools through the real binary and Python mcp 1.30.0**

## Performance

- **Duration:** about 25 min
- **Tasks:** 3
- **Files modified:** 9 (4 created, 5 modified)

## Accomplishments

- list_merge_requests: state defaults to `opened` and is always sent; invalid state rejected in Russian with zero requests; filters sent only when non-empty; one line per MR `!iid state [draft] title source→target @author`.
- get_merge_request: exactly one GET; header with state, draft, author, branches, conflict flag, pipeline (or "pipeline: нет"), file count (or "ещё не известно"), description cut at 1500 runes, web_url and the `detailed_merge_status: <raw> — <advice>` line; checking/unchecked says to repeat in a few seconds.
- mr_status.go holds 24 status advices plus stateAdvice for merged/closed/locked, ready for reuse by merge_merge_request.
- Golden snapshot, stdio e2e and scripts/smoke.py now expect 14 tools; smoke calls both new tools against the fake.

## Task Commits

1. **Task 1: list/get with shared status table** - `66b29f1` (feat)
2. **Task 2: error and edge states for MR reads** - `08702bc` (test)
3. **Task 3: 14 tools through the real binary and Python client** - `590b343` (test)

## Verification (observed)

- `go vet ./...` clean; `go test ./... -count=1` all packages ok (internal/tools, e2e, glclient, config, logging).
- `go test ./e2e -v -run TestPythonSmoke` PASS (not skipped; uv is on PATH) with the real binary and the Python mcp 1.30.0 client, asserting the MR list line and status line in stdout.
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp` succeeded (11.3 MB).
- gofmt: clean on all files once CRLF is normalised (see Deviations).
- Not verified: live gitlab.com behaviour (planned for Phase 4, QA-03).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Duplicate test helper**
- **Found during:** Task 1
- **Issue:** `queryOf` already exists in commits_test.go, so my copy in mr_test.go did not compile.
- **Fix:** Removed my copy and reuse the existing helper.
- **Commit:** 66b29f1

### Environment note (not a plan deviation)

`gofmt -l cmd internal e2e` lists every pre-existing Go file because the Windows working tree checks files out with CRLF (`core.autocrlf=true`); the repository content is LF. New files are LF. The exact verify line `test -z "$(gofmt -l ...)"` therefore fails on pre-existing files independent of this plan; I checked formatting with `tr -d '\r' | gofmt -l`, which reports nothing.

Task 2 found no gap in mr.go: every added behavior test passed against the Task 1 implementation, so no production change was needed in that commit.

## Known Stubs

None.

## Threat Flags

None. All mitigations T-03-01..T-03-05 are covered by tests (state/iid validation before any request, description/title caps, output budget, single GET for checking).

## Self-Check: PASSED

- internal/tools/mr.go, mr_status.go, mr_test.go, mr_status_test.go: present
- commits 66b29f1, 08702bc, 590b343: present in git log
