---
phase: 03-merge-requests
plan: 04
subsystem: api
tags: [gitlab, merge-requests, notes, write, mcp, go, client-go]

requires:
  - phase: 03-merge-requests
    provides: withWrite/op keys, withDraftPrefix, mrSubject, checkIID, smoke loopback guard (plans 01-03)
provides:
  - create_merge_request_note tool (MR pre-read for web_url, single POST, quick-action-safe output)
  - update_merge_request tool (optional fields, state_event close/reopen, Draft prefix, nothing-to-change guard)
  - 19-tool golden, stdio e2e and Python mcp 1.30.0 smoke with note and update writes
affects: [03-05]

tech-stack:
  added: []
  patterns:
    - "Empty value means not passed: options are set only when the trimmed field is non-empty"
    - "A write that needs the current state does one read first; a read error never reaches the write"

key-files:
  created:
    - internal/tools/mr_update_test.go
  modified:
    - internal/tools/mr_notes.go
    - internal/tools/mr_notes_test.go
    - internal/tools/mr.go
    - internal/tools/register.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py

key-decisions:
  - "create_merge_request_note reads the MR once before the POST to get web_url and a clear 404; the write itself is one non-retried POST"
  - "Note body is sent byte for byte; only the emptiness check trims"
  - "A note answer with id 0 is reported as accepted (quick-action-only body), not as note #0"
  - "update_merge_request: draft=true is the Draft: title prefix; draft=false means not passed, so Draft is removed by sending a title without the prefix"
  - "update_merge_request is annotated DestructiveHint=true (close is not additive); create_merge_request_note false"

patterns-established:
  - "Draft-only update on an already-Draft MR costs one GET and sends no PUT"

requirements-completed: [MR-06, MR-08]

duration: 25min
completed: 2026-09-24
---

# Phase 3 Plan 04: create_merge_request_note and update_merge_request Summary

**create_merge_request_note (MR pre-read for the link, one unretried POST, body sent unchanged) and update_merge_request (only non-empty fields, close/reopen, Draft: prefix without blanking), pinned at 19 tools through the real binary and Python mcp 1.30.0**

## Accomplishments

- create_merge_request_note: NormalizeProject, iid and empty-body guards run before any request. One GET of the MR (web_url, and a 404 named "MR (iid) или проект" before anything is written), then one POST with exactly `{"body": ...}`. Output `комментарий #<id> добавлен к MR !<iid>` plus the MR link; id 0 gives `комментарий принят (id не возвращён: ... быстрые команды GitLab)`. 5xx/timeout says `Результат записи неизвестен` with the list_merge_request_notes hint; 403 uses the Phase 2 write wording.
- update_merge_request: state_event validated against close/reopen and a nothing-to-change guard, both before any request. Each option is set only when non-empty. draft=true with a title sends `Draft: <title>` without a read; without a title it reads the MR once and prefixes the current title. An MR already Draft (flag or prefix) with nothing else to change sends no PUT and answers `MR !N уже помечен как Draft; изменений нет`. Output: changed field names, `state:` and `draft:` from the response, branches, link. 5xx says unknown outcome with the get_merge_request hint.
- register.go: both tools added; golden regenerated (19 tools). The tool descriptions state the empty-means-unchanged rule, the Draft removal method and the quick-action warning.
- e2e/smoke: wantNames and EXPECTED_TOOLS have 19 names; the loopback-guarded write block calls both new tools; the Go smoke test adds the POST note and PUT routes and asserts both success lines.

## Task Commits

1. **Task 1: create_merge_request_note** - `df017bd` (feat)
2. **Task 2: update_merge_request** - `29c88ac` (feat)
3. **Task 3: 19 tools through the binary and Python client** - `83ad90b` (test)

## Verification (observed)

- `go vet ./...` clean.
- gofmt clean on the changed Go files (checked through `tr -d '\r'`).
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- `go test ./e2e -v -run 'TestPythonSmoke|TestStdio'`: PASS, TestPythonSmoke not skipped (real binary, Python mcp 1.30.0, note and update executed against the loopback fake).
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: succeeded.
- `grep -En "\*int|\*string|\*bool" internal/tools/mr.go`: no output.
- Not verified: live gitlab.com behaviour of note creation and MR update (planned for Phase 4, QA-03). The quick-action id-0 answer relies on RESEARCH F8/A1, not on a live call.

## Deviations from Plan

For the author to confirm (recorded as required by the plan): create_merge_request_note does one read GetMergeRequest before the note POST. D-11 asks for a single write; the extra GET supplies the MR web_url (GitLab's Note response has none) and a clear 404 before anything is written. The write itself is still a single non-retried POST. Cost: one extra GET per comment.

Process notes:
- TDD tasks were implementation-first with tests right after, so there is no separate failing-test commit.
- In the update output, `draft` appears in the changed-fields list only when a title was actually sent because of draft=true.

## Known Stubs

None.

## Threat Flags

None. T-03-16 (empty means not sent, asserted by body-key tests; nothing-to-change guard), T-03-17 (state_event validated, zero requests on bad value) and T-03-19 (single POST/PUT, unknown-outcome hints) are implemented and tested.

## Self-Check: PASSED

- internal/tools/mr_update_test.go: present
- commits df017bd, 29c88ac, 83ad90b: present in git log
