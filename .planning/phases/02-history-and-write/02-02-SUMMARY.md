---
phase: 02-history-and-write
plan: 02
subsystem: api
tags: [go, mcp, gitlab, commits, diff]

requires:
  - phase: 02-history-and-write (plan 01)
    provides: resolveRef, refLabel, composeList, withSubject, Budget/PageFooter, FakeGitLab test helpers
provides:
  - list_commits tool (ref/path/since/until/author filters, pagination)
  - get_commit tool (header plus budgeted per-file diff with explicit no-patch reasons)
  - shared diff renderer (diffFile, fetchCommitDiff, renderDiffFiles, emptyPatchReason, countPatchLines) for compare_refs and commit_files
affects: [02-03 compare_refs, 02-04 commit_files]

tech-stack:
  added: []
  patterns:
    - "Own JSON decode through client-go NewRequest/Do when a typed struct drops fields the output needs"
    - "Reserve room for all headings before spending budget on bodies, so late items degrade to a note instead of vanishing"

key-files:
  created:
    - internal/tools/commits.go
    - internal/tools/commits_test.go
    - internal/tools/diff.go
    - internal/tools/diff_test.go
  modified:
    - internal/tools/register.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "Follow D-11 (explicit reason) over the RESEARCH inference: commit diff is decoded into own diffFile with collapsed/too_large; inference kept only as the fallback when both flags are false"
  - "Budget accounting reserves the 'патч не показан' suffix for every non-empty file and refunds it when the patch is shown, so the rendered diff stays inside the budget without the final cut dropping headings"

patterns-established:
  - "fetchCommitDiff wire test compares the RequestURI with the one produced by client-go's Commits.GetCommitDiff"

requirements-completed: [READ-06, READ-07]

duration: 25min
completed: 2026-09-24
---

# Phase 2 Plan 02: Commit History and Commit Diff Summary

**list_commits and get_commit over the real binary, with a diff renderer that names the reason for every missing patch (too_large, collapsed, rename, mode, empty/binary) and keeps every file heading inside the 15 000-rune budget.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 3/3
- **Files modified:** 8 (4 created, 4 modified)

## Accomplishments

- `list_commits`: one line per commit ("a1b2c3d4 2026-09-20 Иван Петров Fix login"), default ref is the project's default branch, optional path/since/until/author; since/until are validated as ISO 8601 before any request is made.
- `get_commit`: header (full SHA, author, date, short parents, +/− stats, message capped at 1000 runes, web_url, file count) followed by each file with a patch capped at 2000 runes; files without a patch state why and point to `get_file_contents` with a concrete ref (parent SHA for deleted files).
- `fetchCommitDiff` builds the path with `gitlab.PathEscape` and decodes into `diffFile`, so `collapsed`/`too_large` survive; a wire test proves the RequestURI path equals client-go's own `GetCommitDiff` for `feature/x` and `release-1.0`.
- Real-binary e2e lists 9 tools; the Python `mcp==1.30.0` smoke calls `list_commits` and `get_commit` and sees the `too_large` reason.

## Task Commits

1. **Task 1: list_commits** - `c933143` (feat)
2. **Task 2: get_commit with budgeted diff rendering** - `c37dd95` (feat)
3. **Task 3: e2e and Python smoke for the history slice** - `d8eda34` (test)

## Verification (observed)

- `go vet ./...` exits 0.
- `go test ./... -count=1` exits 0 (e2e, config, glclient, logging, tools all ok).
- `TestPythonSmoke` ran and passed (1.73 s, not skipped): uv present, Python mcp 1.30.0 client drove the built binary.
- `TestToolSchemas` passes for both new tools (name, description <= 900 runes, flat schema, all properties described).
- 30-file budget test: output < 15 500 runes, all 30 headings present, the late ones end with "патч не показан (бюджет вывода исчерпан)".
- gofmt: verified on LF-normalised copies of the touched CRLF files (register.go, stdio_test.go, python_smoke_test.go) and directly on the new LF files (commits.go, commits_test.go, diff.go, diff_test.go): clean. `gofmt -l cmd internal e2e` on the worktree still lists 41 files because the checkout is CRLF (`core.autocrlf=true`); this is pre-existing and not caused by this plan, so the plan's `test -z "$(gofmt -l ...)"` cannot pass literally in this worktree.
- Not verified: real gitlab.com (no live run; hermetic fake only).

## Deviations from Plan

### Auto-fixed Issues

None - no bugs surfaced in the plan's design. One implementation refinement inside the plan's stated algorithm:

**1. [Rule 2 - Correctness] Budget accounting reserves the not-shown suffix and cut marker**
- **Found during:** Task 2 (designing renderDiffFiles)
- **Issue:** The plan reserves only heading runes. Headings that later gain " — патч не показан (...)" and patches that gain the "[патч обрезан: ...]" line would grow past the reserved budget, so the final guard cut could drop late headings, violating "all 30 headings present".
- **Fix:** Reserve the suffix for each file with a patch (refunded when its patch is shown) and keep 80 runes for the cut marker when a patch may be cut.
- **Files modified:** internal/tools/diff.go
- **Commit:** c37dd95

TDD note: tests and implementation were committed together per task (one commit per task), not as separate RED/GREEN commits; the plan is `type: execute`, not `type: tdd`.

## Assumption Drift (advisory)

None material. The plan's expectation that client-go's `NewRequest` accepts an already-escaped relative path held (the wire test confirms identical paths).

## Known Stubs

None.

## Threat Flags

None. Mitigations T-02-06/07/09 are implemented as planned: only `gitlab.PathEscape` on project and sha through `NewRequest` (which rejects foreign hosts), 2000-rune per-file cap plus shared budget plus final guard, `NormalizeRepoPath` and `parseTime` reject bad input before any request.

## Notes for Downstream Plans

- `renderDiffFiles(files, newRef, oldRef, budget)` is ready for `compare_refs` (Plan 03); `countPatchLines` for the per-file stats of `commit_files` (Plan 04).
- Worktree note: an untracked `.planning/HANDOFF.json` may appear; it was not committed.

## Self-Check: PASSED

- Files exist: internal/tools/commits.go, commits_test.go, diff.go, diff_test.go (created and committed).
- Commits exist: c933143, c37dd95, d8eda34.
