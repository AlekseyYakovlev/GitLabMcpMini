---
phase: 03-merge-requests
plan: 03
subsystem: api
tags: [gitlab, merge-requests, write, errors, mcp, go, client-go]

requires:
  - phase: 03-merge-requests
    provides: mrHeader/statusAdvice/mrSubject and MR fixtures from plans 01-02
  - phase: 02-history-and-write
    provides: fetchCompare, resolveRef, withWrite, writeRules, smoke harness
provides:
  - create_merge_request tool (guards, default target, compare pre-check, Draft prefix, single POST)
  - MR-aware write error layer: op keys for all four MR writes, status-aware rules, op-specific unknown-outcome hints
  - 17-tool golden, stdio e2e and Python mcp 1.30.0 smoke with loopback-only write calls
affects: [03-04, 03-05]

tech-stack:
  added: []
  patterns:
    - "writeRule matches on kind + op + status, substrings optional"
    - "unknownOutcomeHint(op) names where to check before repeating a write"
    - "409 duplicate MR is always an error naming the existing !N"

key-files:
  created:
    - internal/tools/mr_create_test.go
  modified:
    - internal/tools/mr.go
    - internal/tools/errors.go
    - internal/tools/errors_test.go
    - internal/tools/register.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py
    - .planning/ROADMAP.md

key-decisions:
  - "Draft is a title prefix (GitLab REST has no draft field); a title already starting with Draft:, [Draft] or (draft) is left as is"
  - "A compare pre-check (one extra read) refuses an MR with no new commits, because GitLab would otherwise create an empty Draft MR; skipped when GitLab reports compare_timeout"
  - "On 409 the existing MR number comes from the GitLab message, else one read-only list by source_branch; the POST is never repeated"
  - "Writes now say 'Результат записи неизвестен' for 5xx, timeout, network, canceled, decode, too large and other; the plain 'повторите позже' is dropped from write texts"
  - "smoke.py runs write calls only when the base URL host is 127.0.0.1, localhost or ::1"

patterns-established:
  - "Where a failed write may have landed, the hint points at the read tool that shows the state (list_merge_requests, get_merge_request, list_merge_request_notes)"

requirements-completed: [MR-05]

duration: 25min
completed: 2026-09-24
---

# Phase 3 Plan 03: create_merge_request and MR write errors Summary

**create_merge_request (compare pre-check, Draft: title prefix, one POST, 409 reported as "открытый MR уже есть: !N") plus a status-aware write error layer with op-specific "outcome unknown" hints, pinned at 17 tools through the real binary and Python mcp 1.30.0 with loopback-only smoke writes**

## Accomplishments

- create_merge_request: guards (empty source, empty title, source equals target, both before and after default-branch resolution) run before any request. Target defaults to the project default branch via resolveRef. One compare read (target to source) refuses an MR with no new commits ("нет изменений между ветками ... commit_files"). Then exactly one POST with only source_branch, target_branch, title and, when given, description. Output: `MR !N создан: src→dst`, draft flag, raw detailed_merge_status with advice, the get_merge_request-before-merge hint, web_url.
- errors.go: op constants for create/update/merge/note; `writeRule.status`; rules with empty substrings match on kind + op + status; 422 texts for same-branch and missing-branch keyed on opCreateMR; `unknownOutcomeHint(op)`; unknown-outcome wording extended to Server, Timeout, Network, Canceled, Decode, TooLarge, Other (backlog 999.2, 02-REVIEW WR-01). Rate limiting (429) stays a plain retry-after text. Read wording is unchanged.
- Duplicate MR: `createMRError` takes `!N` from the 409 message or does one list by source_branch, and always returns an error.
- register.go: create_merge_request with DestructiveHint=false; golden regenerated.
- smoke.py: `is_loopback` guard, all write calls moved under it, new create_merge_request call; the Go smoke test adds the POST route and asserts "MR !5 создан". ROADMAP 999.2 marked closed.

## Task Commits

1. **Task 1: create_merge_request end to end** - `a7e2b31` (feat)
2. **Task 2: MR write error layer** - `6a697d1` (feat)
3. **Task 3: 17 tools, loopback smoke writes, close 999.2** - `a4baabb` (test)

## Verification (observed)

- `go vet ./...` clean.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- `go test ./e2e -v -run 'TestPythonSmoke|TestStdio'`: PASS, TestPythonSmoke not skipped (real binary, Python mcp 1.30.0, writes executed against the loopback fake, "MR !5 создан" in stdout).
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: succeeded.
- gofmt clean on all Go files when checked through `tr -d '\r'`.
- Acceptance greps: no `*int|*string|*bool` in mr.go; 0 status-digit `text:` literals in errors.go; smoke.py parses.
- `is_loopback` logic checked by hand on 127.0.0.1, localhost, [::1] (true) and gitlab.com, 127.0.0.1.evil.com (false); the script itself was exercised only through the loopback fake.
- Not verified: live gitlab.com behaviour (planned for Phase 4, QA-03); the 409 message wording and the compare pre-check behaviour rely on RESEARCH F4/F5, not on a live call.

## Deviations from Plan

None in behavior. Process notes:
- TDD tasks were implementation-first with tests right after, so there is no separate failing-test commit.
- The 409/422/503 create tests belong to Task 2 behavior, so they were added in Task 2's commit rather than Task 1's, to keep each commit green.
- No edits to write_test.go, upsert_test.go or branch_test.go were needed: they only assert "Результат записи неизвестен", which is preserved.

## Assumption Drift (advisory)

- **Found during:** Task 2
- **Planned:** commit 5xx keeps the old text suffix with the changed hint.
- **Actual:** the 5xx write text is now built directly (`<status>: ошибка сервера GitLab.` + unknown outcome + hint) instead of baseText + suffix.
- **Why:** baseText says "повторите позже", which contradicts the unknown-outcome warning on a write (WR-01).

## Known Stubs

None.

## Threat Flags

None. T-03-10 (single POST, 409 as error, compare pre-check), T-03-11 (guards before requests, no double Draft prefix), T-03-13 (capDetail, only !N and web_url extracted) and T-03-14 (loopback guard) are implemented.

## Self-Check: PASSED

- internal/tools/mr_create_test.go: present
- commits a7e2b31, 6a697d1, a4baabb: present in git log
