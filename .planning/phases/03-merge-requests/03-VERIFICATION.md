---
phase: 03-merge-requests
verified: 2026-09-24T00:00:00Z
status: passed
score: 4/4 roadmap success criteria verified (8/8 requirements satisfied)
has_blocking_gaps: false
overrides_applied: 0
deferred:
  - truth: "Behaviour of the MR tools against real gitlab.com (real 405/406/409 merge refusals, real /diffs flags, quick-action note answer)"
    addressed_in: "Phase 4"
    evidence: "Phase 4 SC1: 'Живой прогон на тестовом проекте gitlab.com проходит полный сценарий: ... создание MR, комментарий, merge и очистка'; requirement QA-03 (Pending)"
advisory_warnings:
  - id: WR-01
    summary: "Quick actions (/merge, /close) in note body or MR description bypass the merge_merge_request guard; create_merge_request_note is annotated DestructiveHint=false"
    source: 03-REVIEW.md
  - id: WR-02
    summary: "merge PUT is not pinned to the checked head SHA (D-13 / MRX-03 deferred to v2); the 409 and 'sha must be provided' rules are unreachable while sha is never sent"
    source: 03-REVIEW.md
  - id: WR-03
    summary: "Duplicate-MR fallback lookup filters by source_branch only, may name an MR with a different target"
    source: 03-REVIEW.md
---

# Phase 3: Merge Requests Verification Report

**Phase Goal:** Агент ведёт Merge Request от просмотра до merge: список, детали, diff, комментарии, создание, правка и влитие
**Verified:** 2026-09-24
**Status:** passed
**Re-verification:** No, initial verification

Note: ROADMAP marks the phase `mode: mvp`, but the ROADMAP goal is not in "As a / I want to / so that" form (`user-story.validate` returns false for it). The user story does exist in the PLAN files (plan 03-01 "Phase Goal" section). Verification was done goal-backward against the four ROADMAP success criteria, and the PLAN story outcome ("агент доводит свои изменения из Phase 2 до влитой ветки") is covered by the flow test below.

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | List MRs with state/branch filters and details by `iid`, including `detailed_merge_status` | VERIFIED | `internal/tools/mr.go` `listMergeRequests`: state always sent (default `opened`), invalid state rejected before any request, `source_branch`/`target_branch`/`author_username`/`search` sent only when non-empty, uses real `MergeRequests.ListProjectMergeRequests` and `composeList` footer. `getMergeRequest`: one `GetMergeRequest`, `mrHeader` prints `statusAdvice(mr)` (`detailed_merge_status: <raw> — <advice>`) from `mr_status.go` (24-entry table, unknown value printed raw). Tests `TestListMergeRequestsDefaultsToOpened`, `...SendsFilters`, `TestGetMergeRequestHeader`, `...CheckingIsOneRequest` exist and pass. |
| 2 | Paged per-file MR diff; `collapsed`/`too_large`/`overflow` explicit; empty patch never reads as "no changes"; notes with system marker | VERIFIED | `mr_diff.go`: typed `ListMergeRequestDiffs` with page/per_page, adapter `mrDiffToDiffFile` passes `Collapsed`/`TooLarge` into shared `renderDiffFiles`; `diff.go` `emptyPatchKind` gives explicit reason for too_large, collapsed, rename-only, mode-only, empty/binary. `overflow`: GitLab `/diffs` has no such field, so it is derived from `changes_count` ending in `+` ("часть файлов не вернулась ...", `mrDiffsHeader`); documented in 03-02-SUMMARY as a surrogate. Empty page text distinguishes "diff ещё готовится" from "изменений нет". `mr_notes.go` `noteLines` prefixes `[system] ` for `n.System`. Tests `TestGetMergeRequestDiffsEmptyPatchReasons`, `...OverflowLine`, `...Paging`, `TestListMergeRequestNotes*` pass. |
| 3 | Create MR from a branch, add a general comment, change title/description/state | VERIFIED | `createMergeRequest` (guards, default target via `resolveRef`, compare pre-check, `Draft:` prefix, one `CreateMergeRequest`, 409 mapped to "открытый MR уже есть: !N"); `createMergeRequestNote` (one pre-read GET, one `CreateMergeRequestNote` POST); `updateMergeRequest` (only non-empty fields, `state_event` close/reopen validated, nothing-to-change guard, one `UpdateMergeRequest` PUT). All registered in `register.go`. Tests `TestCreateMergeRequest*`, `TestCreateMergeRequestNote*`, `TestUpdateMergeRequest*` exist and pass; write-once behaviour pinned on 5xx. |
| 4 | `merge_merge_request` checks `detailed_merge_status`; on refusal (405/406/409, failed checks) explains instead of raw error | VERIFIED | `mr_merge.go`: GET MR, `stateAdvice` refusal for non-open, then refusal unless `detailed_merge_status == "mergeable"` (plain error, no PUT), then exactly one `AcceptMergeRequest`. `errors.go` op-keyed `opMergeMR` rules: 405/406 ("слияние отклонено ... Вызовите get_merge_request"), 409 (source changed), 422 "branch cannot be merged", 400 "sha must be provided", 401, 403. `glclient.classifyResponse` maps every 4xx to `KindBadRequest` (401/403/404/429 have own kinds), so the status-keyed rules are reachable. Tests `TestMergeMergeRequestRefusedBeforePut`, `...PutFailures`, `TestMergeRequestFlow` (create, note, checking, mergeable, merge, merged; exactly one POST create, one POST note, one PUT merge) pass. |

**Score:** 4/4 truths verified

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|--------------|----------|
| 1 | Real gitlab.com confirmation of merge refusal codes/wording, `/diffs` flags, quick-action note answer | Phase 4 | Phase 4 SC1 and QA-03: full live scenario incl. MR, comment, merge. All live-behaviour claims in 03-0x-SUMMARY are marked "not verified: live" by the executor. |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/tools/mr.go` (482 lines) | list/get/create/update | VERIFIED | Substantive, wired via `register.go` |
| `internal/tools/mr_status.go` (75) | status/state advice table | VERIFIED | Used by get, create, merge |
| `internal/tools/mr_diff.go` (132) | MR diffs | VERIFIED | Wired; feeds shared `renderDiffFiles` |
| `internal/tools/mr_notes.go` (152) | list + create note | VERIFIED | Wired |
| `internal/tools/mr_merge.go` (110) | merge | VERIFIED | Wired |
| `internal/tools/errors.go` | MR write rules | VERIFIED | `opCreateMR/opUpdateMR/opMergeMR/opMRNote` rules present |
| `internal/tools/register.go` | 8 new tools registered | VERIFIED | All 8 `AddTool` entries present with annotations |
| `internal/tools/testdata/tools_list.golden.json` | 20-tool snapshot | VERIFIED | 20 `"name"` entries |
| `e2e/*`, `scripts/smoke.py` | binary + Python mcp smoke | VERIFIED | `TestPythonSmoke` re-run: PASS (not skipped) |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `getMergeRequest`, `createMergeRequest`, `mergeMergeRequest` | `statusAdvice`/`stateAdvice` | direct calls | WIRED |
| `register.go` | all 8 MR handlers | `mcp.AddTool(... safe(d, handler))` | WIRED |
| MR write handlers | `withWrite(op...)` rules | `errors.go` op constants | WIRED |
| `mrDiffToDiffFile` | `renderDiffFiles` | shared renderer with Phase 2 | WIRED |

### Data-Flow Trace (Level 4)

Handlers render values taken from typed client-go responses (`MergeRequests.*`, `Notes.*`), not static values. `TestMergeRequestFlow` drives a stateful fake through the whole chain over the in-memory MCP session and asserts the observed outputs. FLOWING.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Build | `go build ./...` | clean | PASS |
| Static | `go vet ./...` | clean | PASS |
| Format | `tr -d '\r' \| gofmt -l` over cmd/internal/e2e | no files listed | PASS |
| All tests | `go test ./... -count=1` | e2e, config, glclient, logging, tools all ok | PASS |
| Real binary + Python mcp 1.30.0 | `go test ./e2e -run TestPythonSmoke -v -count=1` | PASS | PASS |
| Tool count | `grep -c '"name"' tools_list.golden.json` | 20 | PASS |

### Probe Execution

No probes declared. SKIPPED.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| MR-01 | 03-01 | list_merge_requests with state/branch filters | SATISFIED | `listMergeRequests`, tests |
| MR-02 | 03-01 | get_merge_request with detailed_merge_status | SATISFIED | `getMergeRequest`, `mrHeader` |
| MR-03 | 03-02 | paged diff, collapsed/too_large/overflow explicit | SATISFIED | `getMergeRequestDiffs` (overflow via changes_count surrogate) |
| MR-04 | 03-02 | notes with system marked | SATISFIED | `noteLines` `[system]` |
| MR-05 | 03-03 | create_merge_request | SATISFIED | `createMergeRequest` |
| MR-06 | 03-04 | create_merge_request_note | SATISFIED | `createMergeRequestNote` |
| MR-07 | 03-05 | merge with status check and clear refusals | SATISFIED | `mergeMergeRequest`, opMergeMR rules |
| MR-08 | 03-04 | update_merge_request | SATISFIED | `updateMergeRequest` |

All 8 IDs declared in the PLAN `requirements-completed` fields (01: MR-01/02, 02: MR-03/04, 03: MR-05, 04: MR-06/08, 05: MR-07) map to REQUIREMENTS.md and to Phase 3 in its traceability table. No orphaned requirements.

### Anti-Patterns Found

No TBD/FIXME/XXX/TODO/HACK markers in the phase's Go files or `scripts/smoke.py`. No stubs: no static returns, all handlers reach a client call.

### Advisory Warnings (from 03-REVIEW.md, re-assessed)

None of these make a ROADMAP success criterion false, so they are not blocking gaps. They are recorded for a developer decision (candidates for `/bm:code-review-fix` or backlog).

- **WR-01 (worth deciding before real use):** confirmed in code. `create_merge_request_note` sends `in.Body` unchanged and is `DestructiveHint: false`, so a `/merge` line in a note (or in an MR description read via `get_merge_request`, i.e. prompt injection) merges without the `mergeable` pre-check. The tool descriptions mention quick actions but do not enforce anything.
- **WR-02:** confirmed. `mr_merge.go` sends no `sha` (explicit decision D-13, MRX-03 v2, code comment says so). The 409 and "sha must be provided" rules are therefore reachable only if GitLab sends them for other reasons; the executor raised this as an open question in 03-05-SUMMARY.
- **WR-03:** confirmed. `createMRError` fallback lookup at `mr.go:378-382` omits `TargetBranch`. Only affects the wording of the 409 error when GitLab's message carries no `!N`.

### Human Verification Required

None for this phase. Live gitlab.com behaviour is explicitly scoped to Phase 4 (QA-03) and listed under Deferred Items.

### Gaps Summary

No gaps. All four ROADMAP success criteria are implemented, wired, and covered by passing tests including a full create-to-merged flow and the real-binary Python mcp 1.30.0 smoke. SUMMARY claims were cross-checked against code and a fresh test run. Residual risk is limited to unverified live GitLab behaviour (Phase 4) and the three advisory review warnings above.

---

_Verified: 2026-09-24_
_Verifier: Claude (gsd-verifier)_
