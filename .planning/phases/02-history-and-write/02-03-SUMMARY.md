---
phase: 02-history-and-write
plan: 03
subsystem: api
tags: [go, mcp, gitlab, compare, diff]

requires:
  - phase: 02-history-and-write (plan 02)
    provides: diffFile, renderDiffFiles, commitLine, diffBodySlack, Budget/PageFooter/withSubject
provides:
  - compare_refs tool (commits section, server-side file paging, honest diff states)
  - fetchCompare with own compareResult decode (keeps collapsed/too_large)
affects: [02-04 commit_files, 03 merge requests]

tech-stack:
  added: []
  patterns:
    - "Server-side slicing of a full GitLab list into pages, with a synthetic gitlab.Response{NextPage} feeding the shared PageFooter"

key-files:
  created:
    - internal/tools/compare.go
    - internal/tools/compare_test.go
  modified:
    - internal/tools/register.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "compare response decoded into own compareResult (D-11), same approach as get_commit; no Straight parameter (deferred idea)"
  - "compare_refs lists 10 tools total in the real binary; the smoke script calls it only in hermetic mode"

patterns-established:
  - "fetchCompare wire test compares the RequestURI with the one produced by client-go's Repositories.Compare"

requirements-completed: [UTIL-02]

duration: 15min
completed: 2026-09-24
---

# Phase 2 Plan 03: compare_refs Summary

**compare_refs compares two branches, tags or commits: header "from → to", up to 20 commit lines, files paged on the server side and rendered by the shared diff renderer, with explicit compare_timeout, same-ref and out-of-range-page messages.**

## Accomplishments

- `compare_refs` (ReadOnlyHint): header `сравнение <project>: <from> → <to>`, `коммитов: N` with at most 20 `commitLine` lines and `(показаны первые 20 из N)`, `файлов: M, на странице: lo-hi`, then the files via `renderDiffFiles(slice, to, from, budget)`.
- File paging is sliced on the server side (GitLab compare returns all files); the shared `PageFooter` gets a synthetic `gitlab.Response{NextPage}` when more files remain. A page past the end reports `на странице N файлов нет: всего файлов M` instead of looking like "no changes".
- `compare_timeout` yields "предупреждение: ... diff может быть неполным" directly under the header; `compare_same_ref` yields "from и to указывают на один и тот же коммит: различий нет" and stops.
- `fetchCompare` uses `gitlab.PathEscape` for the project and `CompareOptions` for `from`/`to`, so the ref values only travel as encoded query values (T-02-12). The wire test pins `/api/v4/projects/g%2Fp/repository/compare?from=main&to=feature%2Fx` and asserts it is identical to `Repositories.Compare`.
- The real binary lists 10 tools; the Python `mcp==1.30.0` smoke calls `compare_refs` and sees the timeout warning.

## Task Commits

1. **Task 1: compare_refs with server-side file paging and honest diff states** - `46cf0ac` (feat)
2. **Task 2: e2e and Python smoke for compare_refs** - `8c419dd` (test)

## Verification (observed)

- `go vet ./...` exits 0.
- `go test ./... -count=1` exits 0 (e2e, config, glclient, logging, tools all ok).
- `TestPythonSmoke` ran and passed (2.80 s, not skipped), including the `diff может быть неполным` assertion.
- `TestToolSchemas/compare_refs` passes (description <= 900 runes, flat schema, described properties).
- `TestFetchCompareWireMatchesClientGo`, paging (pages 1, 3, 4 of 45 files), commit cap (25 -> 20), timeout, same-ref, too_large hints (ref=feature/x for new path, ref=main for deleted), empty from/to (zero requests), 404 (mentions 404 and ref), 30-file budget: all pass.
- Acceptance grep: `Straight` does not appear in compare.go.
- gofmt: the worktree is CRLF (`core.autocrlf=true`), so `gofmt -l` lists CRLF files regardless of content. Verified with `tr -d '\r' | gofmt -l` on register.go, stdio_test.go, python_smoke_test.go (clean) and directly on the new LF files compare.go, compare_test.go (clean). The plan's literal `test -z "$(gofmt -l cmd internal e2e)"` cannot pass in this worktree for that pre-existing reason.
- Not verified: real gitlab.com (hermetic fake only).

## Deviations from Plan

None - plan executed exactly as written. Tests and implementation were committed together in one commit per task (plan type is `execute`, not `tdd`).

## Assumption Drift (advisory)

None material.

## Known Stubs

None.

## Threat Flags

None. T-02-11 (8 MiB body cap from the shared client, per_page <= 100, 2000-rune patch cap, 15 000-rune budget with final guard) and T-02-12 (PathEscape plus query encoding, wire test) are implemented as planned.

## Self-Check: PASSED

- Files exist: internal/tools/compare.go, internal/tools/compare_test.go.
- Commits exist: 46cf0ac, 8c419dd.
