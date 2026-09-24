---
phase: 03-merge-requests
plan: 05
subsystem: api
tags: [gitlab, merge-requests, merge, mcp, go, client-go]

requires:
  - phase: 03-merge-requests
    provides: statusAdvice/stateAdvice table, withWrite/op keys, unknownOutcomeHint, MR create/note/update tools, smoke loopback guard (plans 01-04)
provides:
  - merge_merge_request tool (GET pre-check gate, optional fields, one non-retried PUT, Russian success text)
  - opMergeMR write rules (400 sha, 401/403 permission, 405/406 state change, 409 source changed, 422 unmergeable)
  - stateful MR flow test (create, note, checking, mergeable, merge, merged)
  - 20-tool golden, stdio e2e and Python mcp 1.30.0 smoke including the merge
affects: [phase-04-live-verification]

tech-stack:
  added: []
  patterns:
    - "Irreversible write: read, refuse with a plain error unless the state allows it, then exactly one PUT"
    - "Merge 401 is worded as a permission problem, not a bad token, keyed on op"

key-files:
  created:
    - internal/tools/mr_merge.go
    - internal/tools/mr_merge_test.go
    - internal/tools/mr_flow_test.go
  modified:
    - internal/tools/errors.go
    - internal/tools/errors_test.go
    - internal/tools/register.go
    - internal/tools/testdata/tools_list.golden.json
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py
    - .planning/ROADMAP.md

key-decisions:
  - "merge_merge_request refuses (plain error, zero PUT) when state is not opened, then when detailed_merge_status is not mergeable; stateAdvice runs first because a merged MR reports not_open"
  - "squash, should_remove_source_branch and merge_commit_message are sent only when set; false and blank mean not passed; sha is never sent (D-13, MRX-03)"
  - "Success text says removal of the source branch was requested, never that it was deleted (GitLab deletes it asynchronously)"
  - "Merge 401 means no right to merge, not an invalid token; 405 and 406 share one wording"
  - "If the PUT answer carries no iid, the success text falls back to the requested iid"

patterns-established:
  - "Test-side stateful fake with fake.Handle drives a whole tool chain over the in-memory MCP session"

requirements-completed: [MR-07]

duration: 30min
completed: 2026-09-24
---

# Phase 3 Plan 05: merge_merge_request and the full MR flow Summary

**merge_merge_request with a GET pre-check that reuses the shared status table, one unretried PUT without sha, op-keyed Russian explanations for every documented merge refusal, a stateful create-to-merged flow test, and 20 tools pinned through the real binary and Python mcp 1.30.0**

## Accomplishments

- merge_merge_request: NormalizeProject and iid checks first, then one GET. A non-open MR gets the stateAdvice text (merged, closed with the reopen hint, locked); an open MR whose detailed_merge_status is not `mergeable` gets `слияние не выполнено: detailed_merge_status: ... — <advice>`. Both are plain errors, so no PUT is sent. Otherwise one PUT with only the set options. Output: `MR !N влит (state=merged)`, the merge commit (fallback to squash commit, then head sha, then `-`), `удаление ветки-источника: запрошено/не запрошено`, web_url. A response whose state is not merged says the merge is not finished and points to get_merge_request. Registered with DestructiveHint true; the description is under 900 runes (asserted).
- errors.go: opMergeMR rules placed ahead of the generic rules: 405 and 406 (`слияние отклонено ... Вызовите get_merge_request`), 409 (source changed), 422 "branch cannot be merged", 400 "sha must be provided" (names MRX-03), 401 (`нет прав вливать этот MR`, no "token invalid" wording), 403 (protected target branch or role, scope api). 5xx, timeout and network keep the unknown-outcome text with the get_merge_request hint. Other ops are unaffected (asserted: 405 on create_merge_request stays generic, 403 on commit keeps the Phase 2 text).
- Tests: 14 new errors_test cases; mr_merge_test covers the defaults (empty PUT body, no sha), set options, false and blank not sent, commit-line fallbacks, not-merged-yet, five pre-check refusals with zero PUT, 404, iid 0, and PUT 405/503/401 with exactly one PUT. TestMergeRequestFlow chains create, note, get (checking), get (mergeable), merge, get (merged) with exactly one POST create, one POST note and one PUT merge.
- e2e: wantNames, EXPECTED_TOOLS and the golden have 20 tools; the loopback-guarded smoke block calls merge_merge_request; the Go smoke test adds the PUT merge route and asserts `MR !5 влит`. ROADMAP 999.5 records the implemented loopback write guard (heading untouched).

## Task Commits

1. **Task 1: merge_merge_request** - `6e1947c` (feat)
2. **Task 2: merge refusal wording and the flow test** - `e0385fc` (feat)
3. **Task 3: 20 tools through the binary and Python client, 999.5 note** - `5ec68bc` (test)

## Verification (observed)

- `go vet ./...` clean.
- gofmt: no files listed for cmd, internal, e2e (checked through `tr -d '\r'`).
- `go test ./... -count=1`: ok for e2e, config, glclient, logging, tools.
- `go test ./e2e -v -run 'TestPythonSmoke|TestStdio'`: TestPythonSmoke, TestStdioHandshakeAndWhoami, TestStdioMissingToken, TestStdioWirePaths all PASS (TestPythonSmoke not skipped: real binary, Python mcp 1.30.0, merge executed against the loopback fake).
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: succeeded.
- Greps: `SHA:` in mr_merge.go (non-comment lines) 0; `удалена` 0; golden has 20 `"name"` entries.
- Not verified: live gitlab.com merge behaviour and the real status codes GitLab returns on merge refusals (405/406/409/422 wording rests on RESEARCH F6, not a live call). Planned for Phase 4 (QA-03).

## Open question for the author

**Send the pre-checked `mr.SHA` as `sha` on merge?** (RESEARCH Open Question 1.) The handler already holds `mr.SHA` from the pre-check GET. Sending it would close the race between the GET and the PUT (GitLab would then answer 409 if the source branch moved) and would satisfy a group or instance "require SHA" setting. D-13 defers sha protection to v2 (MRX-03), so it is NOT sent; the 400 "SHA must be provided" answer is mapped to a clear text instead. If approved, the change is one line in `internal/tools/mr_merge.go` (`SHA: gitlab.Ptr(mr.SHA)` in the options) plus one test asserting the body key and updating the 400 wording.

## Deviations from Plan

None in behaviour. Small additions and process notes:

- The success text falls back to the requested iid if GitLab's PUT answer has no iid (defensive, Rule 2).
- TDD tasks were implementation-first with tests right after, so there is no separate failing-test commit (same as plans 03-01 to 03-04).
- The task-3 e2e test edit briefly produced a non-compiling Go string (a helper script turned a `\n` escape into a real newline); it was found by `go vet` and fixed before the commit.

## Known Stubs

None.

## Threat Flags

None. T-03-21 (pre-check gate, asserted zero PUT on every refusal, DestructiveHint true), T-03-22 (one PUT even on 503, unknown-outcome text), T-03-24 (401/403 worded as permission), T-03-25 and T-03-26 (op-keyed rules, options sent only when set) are implemented and tested. T-03-23 (GET-to-PUT race) is accepted per D-13 and carried as the open question above.

## Self-Check: PASSED

- internal/tools/mr_merge.go, mr_merge_test.go, mr_flow_test.go: present
- commits 6e1947c, e0385fc, 5ec68bc: present in git log
