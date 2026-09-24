---
phase: 01-connect-and-read
plan: 04
subsystem: api
tags: [go, mcp, gitlab, client-go-v2, pagination]

requires:
  - phase: 01-connect-and-read
    provides: "tools.Deps/safe/withSubject/toToolText, glclient.NormalizeProject, testutil fake GitLab and in-memory MCP session"
provides:
  - "Shared output budget and pagination footer (format.go) for every list tool"
  - "list_projects tool (READ-01)"
  - "get_project tool (READ-02)"
  - "ProjectIn input struct reusable by later tools"
affects: [01-05, 01-06, phase-02, phase-03]

tech-stack:
  added: []
  patterns:
    - "composeList: budgeted body, then TruncatedFooter, then page footer"
    - "Handlers return text; safe() converts errors and redacts"
    - "Tests use the in-memory MCP session against the fake GitLab and assert raw request URIs"

key-files:
  created:
    - internal/tools/format.go
    - internal/tools/format_test.go
    - internal/tools/projects.go
    - internal/tools/projects_test.go
  modified:
    - internal/tools/register.go

key-decisions:
  - "Budget drops the newline at the cut so footers are appended on their own line by composeList"
  - "Added composeList helper (not in the plan interface list) so all list tools share one composition path"
  - "list_projects omits branch and description parts when empty, no trailing dash"

patterns-established:
  - "Pagination hint from X-Next-Page / Link rel=next only; total counters never read"
  - "Tool input descriptions via jsonschema tags, plain non-pointer field types"

requirements-completed: [READ-01, READ-02, FND-04, FND-06, FND-08]

duration: 12min
completed: 2026-09-24
---

# Phase 1 Plan 04: list_projects and get_project Summary

**Two read-only project tools plus a shared rune-based output budget and page footer, verified through the in-memory MCP session against a fake GitLab.**

## Accomplishments

- `format.go`: `OutputBudget`, `TruncatedFooter`, `Budget` (rune-based, cuts on a line boundary), `ClampPaging`, `PageFooter`, `oneLine`, and the `composeList` helper.
- `list_projects`: membership-only by default, `include_public`, `search`, `page`/`per_page` clamped to 1..100, one line per project, page footer from `X-Next-Page`/`Link`.
- `get_project`: accepts a numeric ID or `group/subgroup/project`, sends the encoded path `/api/v4/projects/group%2Fsub%2Fmy%2Eproj`, rejects URLs before any request, reports an empty repository explicitly.

## Task Commits

1. Task 1: Shared output budget and pagination footer - `e1a5cba`
2. Task 2: list_projects and get_project tools - `82ad6a5`

## Verification (observed)

- `go test ./internal/tools/ -run 'Budget|ClampPaging|PageFooter|OneLine|Compose' -count=1 -v`: all 9 tests PASS.
- `go test ./internal/tools/ -count=1 -v -run 'Project|ToolSchemas'`: 10 project tests and TestToolSchemas (whoami, list_projects, get_project) PASS.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- `go vet ./...`: clean.
- Greps: no `X-Total|TotalItems|TotalPages` and no pointer types in `internal/tools/projects.go`.
- Not verified: behaviour against real gitlab.com (no live token in this run); `simple=true` omitting `default_branch` (RESEARCH A2) was handled but not observed live.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Package comment wording**
- **Found during:** Task 1
- **Issue:** The acceptance grep for `X-Total|TotalItems|TotalPages` would match a comment that named the header.
- **Fix:** Reworded the comment to say "total counters" instead.
- **Files modified:** internal/tools/format.go
- **Commit:** e1a5cba

### Notes

- TDD RED commit was not made separately: tests and implementation were written together and committed as one feat commit per task. Tests were executed and pass.
- `gofmt -l` lists many pre-existing files in the worktree (CRLF working-copy conversion by git autocrlf); the new files were written with LF and are not flagged apart from `register.go`, which inherits the existing CRLF line endings. Not touched as out of scope.

## Known Stubs

None.

## Threat Flags

None. T-01-16 (per_page clamp, budget, oneLine caps), T-01-17 (NormalizeProject rejects URLs, verified by test), T-01-19 (errors via toToolText, safe() redacts) are implemented.

## Self-Check: PASSED

- internal/tools/format.go, format_test.go, projects.go, projects_test.go: present
- Commits e1a5cba and 82ad6a5 exist
