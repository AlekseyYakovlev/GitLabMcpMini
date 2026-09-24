---
phase: 02-history-and-write
plan: 01
subsystem: api
tags: [go, mcp, gitlab, branches, write-errors, fake-gitlab]

requires:
  - phase: 01-read-tools
    provides: safe() handler wrapper, resolveRef, composeList/PageFooter, withSubject/toToolText, FakeGitLab, e2e and Python smoke harness
provides:
  - list_branches tool (READ-05)
  - create_branch tool (WRT-01)
  - write-aware error wording (withWrite, opCreateBranch/opCommit, ordered writeRules, statusPrefix, baseText)
  - FakeGitLab.Recorded() request-body capture
  - 7-tool e2e and Python mcp 1.30.0 smoke
affects: [02-02, 02-03, 02-04, commit tools, merge request tools]

tech-stack:
  added: []
  patterns:
    - "Write errors are labelled with withWrite(op, subject, err); wording rules key on the op, not the subject text"
    - "Write rule texts carry no status digits; the real HTTP status is prepended by statusPrefix"
    - "5xx/timeout/network on a write appends 'Результат записи неизвестен'; POST is never retried"

key-files:
  created:
    - internal/tools/branch.go
    - internal/tools/branch_test.go
  modified:
    - internal/tools/errors.go
    - internal/tools/errors_test.go
    - internal/tools/register.go
    - internal/testutil/fakegitlab.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "Rule text placeholders: writeRule.text uses a single %s for the capped GitLab detail instead of an extra struct field, keeping the writeRule shape from the plan"
  - "create_branch annotated DestructiveHint=false (additive write); list_branches ReadOnlyHint=true"

patterns-established:
  - "withWrite + writeRules table: later commit/file plans append rules keyed on opCommit"
  - "Recorded() gives tests the exact method, raw URI and JSON body of every request"

requirements-completed: [READ-05, WRT-01]

duration: 25min
completed: 2026-09-24
---

# Phase 2 Plan 01: Branches (list_branches, create_branch) Summary

**list_branches and create_branch over client-go v2 with write-aware Russian error wording (real HTTP status, "Результат записи неизвестен" on ambiguous failures), request-body capture in the fake GitLab, and a 7-tool real-binary plus Python mcp 1.30.0 smoke.**

## Performance

- **Tasks:** 3/3
- **Files modified:** 9 (2 created)

## Accomplishments

- `list_branches`: one line per branch, `name shortSHA [default] [protected] [merged] title`; optional `search`, `page`/`per_page`; footer announces a next page also when GitLab answers with only a `Link rel="next"` header (covered by a fake test). A branch with `commit: null` renders `name -`.
- `create_branch`: explicit `ref` sends a single POST with no project GET; empty `ref` resolves the project default branch through `resolveRef` and the output says "(ветка по умолчанию)". Blank branch name is rejected before any request. Output: name, source ref, full tip SHA, web_url.
- Write error wording: branch exists (`<real status>: ветка уже существует`, works for 400 and 409), invalid ref (400 and 422), invalid branch name, 403 (protected branch / scope `api` / role Developer+), 401 (scope `api`), 5xx/timeout/network with "Результат записи неизвестен". The branch-exists rule is keyed on `opCreateBranch`, so `opCommit` falls through to the generic 400 text. A 404 on create_branch reads "не найдено (проект или ветка)".
- SC-2 test: stateful fake, `create_branch feature/new` then `list_branches` shows it.
- `FakeGitLab.Recorded()` returns method, raw URI and body for each request; `Requests()` output unchanged; `Reset()` clears both.

## Task Commits

1. **Task 1: list_branches and create_branch** - `7c031bb` (feat)
2. **Task 2: write-aware error wording** - `c1a67c7` (feat)
3. **Task 3: real binary and Python mcp 1.30.0 smoke** - `d28516d` (test)

## Verification (observed)

- `go vet ./...` exit 0.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- `go test ./e2e -run TestPythonSmoke -v`: PASS in 0.72s (not skipped; uv present), so the real binary lists 7 tools and the Python `mcp` 1.30.0 client called `list_branches` and `create_branch` against the fake GitLab.
- Tool-level tests observed passing: exactly 1 POST on a 503, "400: ветка уже существует" and "409: ветка уже существует", Link-only next-page footer.
- gofmt: the working tree is checked out with CRLF (`core.autocrlf=true`, index is LF), so `gofmt -l` lists every file in the repo including untouched ones; the new `.go` files written with LF (`branch.go`, `branch_test.go` etc.) report no gofmt diffs. No real gitlab.com call was made (offline plan).

## Deviations from Plan

None - plan executed exactly as written. One small implementation choice: rule texts that show the GitLab detail use a `%s` placeholder in `writeRule.text` (Sprintf with the capped detail) rather than adding a struct field, so the `writeRule` shape matches the interfaces block.

## Assumption Drift (advisory)

None.

## Carried to Phase 4

- list_branches pagination on gitlab.com: GitLab may answer with keyset `Link rel=next` and ignore `page`; Link-only answer is covered by the PageFooter fallback and a fake test; the live run must confirm that `page=2` returns the second page (or record keyset behaviour and adjust the hint).

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat model (T-02-01..05 mitigations implemented: NormalizeProject, JSON-body-only branch/ref, empty branch rejected, single POST, capped detail, per_page clamp and output budget).

## Self-Check: PASSED

- Files found: internal/tools/branch.go, internal/tools/branch_test.go, internal/tools/errors.go, internal/testutil/fakegitlab.go, scripts/smoke.py
- Commits found: 7c031bb, c1a67c7, d28516d
