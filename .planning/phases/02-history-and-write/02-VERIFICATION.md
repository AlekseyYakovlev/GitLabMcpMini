---
phase: 02-history-and-write
verified: 2026-09-24T00:00:00Z
status: gaps_found
score: 4/4 must-haves verified
has_blocking_gaps: false
overrides_applied: 0
gaps:
  - truth: "commit_files: create/update require content (schema/description say 'required for create and update')"
    status: partial
    severity: minor
    reason: >
      CR-01 confirmed in code. ActionIn.Content is a plain string with omitempty and the hand-written
      schema lists only action and file_path as required. toFileAction sets sendContent=true for
      create/update unconditionally, so an action without a content key commits an EMPTY file and
      reports success. This does not break any of the 4 ROADMAP success criteria (they require that
      multi-file create/update/delete works, that the nested actions[] schema is accepted by client
      1.30.x, and that a commit is visible via get_commit - all verified), but it is a silent
      destructive write on a DestructiveHint tool caused by a plausible model mistake.
    artifacts:
      - path: "internal/tools/write.go"
        issue: "lines ~59-64 (ActionIn.Content string) and ~130-134 (sendContent = true for create/update) - presence of content is not detectable"
      - path: "internal/tools/schema.go"
        issue: "content not in items.required, while its description says required for create/update"
    missing:
      - "Make content a *string (hand-written schema means no nullable leak), reject create/update with content == nil, allow \"\" for an intentionally empty file"
      - "Test: create and update actions without a content key send no request"
  - truth: "Write-failure wording says the write may have been applied for every failure kind where the POST may have landed"
    status: partial
    severity: minor
    reason: "WR-01 confirmed in errors.go writeText: only Server/Timeout/Network get writeUnknownOutcome; Canceled/Decode/TooLarge/Other read as 'nothing happened' and invite a duplicate commit. SC4's understandable-message requirement is met for the named cases (exists / not exists / protected / scope); this is an edge-case wording gap."
    artifacts:
      - path: "internal/tools/errors.go"
        issue: "switch in writeText lacks Canceled, Decode, TooLarge, Other"
    missing:
      - "Add those kinds to the writeUnknownOutcome case"
  - truth: "create_or_update_file protection against overwriting others' edits reflects what the model has seen"
    status: partial
    severity: minor
    reason: "WR-02 confirmed: the tool re-reads the file itself and sends that fresh last_commit_id; UpsertFileIn has no last_commit_id input, so the description's 'protects against overwriting foreign edits' overstates it. Behaviour matches the plan (02-05 GET-based create/update), only the description is misleading."
    artifacts:
      - path: "internal/tools/write.go"
        issue: "createOrUpdateFileDescription overstates protection; no last_commit_id input"
    missing:
      - "Correct the description, or add an optional last_commit_id input"
  - truth: "compare_refs page handling is safe for any page value"
    status: partial
    severity: minor
    reason: "WR-03 confirmed by reading compare.go: lo=(page-1)*perPage overflows for huge page and slices Diffs[lo:hi] with negative bounds; recovered by safe() as an 'internal error', server does not crash."
    artifacts:
      - path: "internal/tools/compare.go"
        issue: "unbounded page arithmetic"
    missing:
      - "Guard page > total/perPage+1 before computing lo/hi"
  - truth: "smoke.py never performs writes against real gitlab.com"
    status: partial
    severity: minor
    reason: "WR-04 (from review, not re-run): write steps gated only by 'if base_url'. Relevant to Phase 4 live run, not to Phase 2 goal."
    artifacts:
      - path: "scripts/smoke.py"
        issue: "hermetic mode decided by presence of --base-url"
    missing:
      - "Gate writes on loopback host or an explicit --allow-writes flag before Phase 4 live run"
---

# Phase 2: History and write - Verification Report

**Phase Goal:** Агент изучает ветки, коммиты и различия и вносит изменения в репозиторий: создаёт ветку и коммитит один или несколько файлов
**Verified:** 2026-09-24
**Status:** gaps_found (only minor gaps; no ROADMAP success criterion is broken)
**Re-verification:** No - initial verification
**Mode:** mvp in ROADMAP, but the goal is not in "As a ... I want to ... so that ..." form; verified goal-backward against the 4 ROADMAP Success Criteria instead (see note below).

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Branch list, commits by ref/path, one commit with diff, compare_refs, with pagination and no silent truncation | VERIFIED | `branch.go` listBranches (ClampPaging + composeList footer), `commits.go` (Ref/Path/Since/Until options passed to ListCommits, get_commit renders diff via fetchCommitDiff + renderDiffFiles with explicit too_large/collapsed reasons), `compare.go` (own compareResult keeps collapsed/too_large/compare_timeout/compare_same_ref, server-side file paging, `TruncatedFooter` when Budget cuts, PageFooter). All 4 tools registered in `register.go`, present in `testdata/tools_list.golden.json`. |
| 2 | create_branch from a given ref, branch then visible in list_branches | VERIFIED | `createBranch` resolves ref (default = project default), single POST via Branches.CreateBranch, errors via withWrite(opCreateBranch); rule "ветка уже существует" is op-scoped. list_branches exists and reads the same API; fake-GitLab e2e covers the pair (e2e/stdio_test.go, `go test ./...` green). |
| 3 | One commit creating/changing/deleting several files (commit_files, actions[], no force) and visible via get_commit; nested actions[] schema accepted by 1.30.x client | VERIFIED (with CR-01 caveat) | `write.go` commitFiles: validation, single `Commits.CreateCommit` POST (no `force`/`start_branch` in options), actions create/update/delete/move, response carries full SHA for get_commit. Hand-written `schema.go` schema: plain array of objects, no `$ref`/`anyOf`/null, `additionalProperties:false` on both levels. `TestPythonSmoke` (Python mcp 1.30.0 client via stdio) ran and PASSED in this verification (not skipped). CR-01: an omitted `content` on create/update commits an empty file - a robustness defect, not a capability failure; see Gaps. |
| 4 | create_or_update_file; clear messages for exists / not exists / protected branch / insufficient scope; write requests never auto-retried | VERIFIED | `createOrUpdateFile` GETs the file on the branch: 404 -> create, found -> update with last_commit_id, other read errors -> no POST. `errors.go` writeRules: file already exists, file doesn't exist, "not allowed to push" -> protected-branch text, "changed since you started editing", 403 text mentions branch protection / scope `api` / role, 401 adds scope `api` hint. `glclient/retry.go` checkRetry only retries GET/HEAD; tests assert exactly one POST. |

**Score:** 4/4 ROADMAP truths verified.

### CR-01 vs. success criteria

CR-01 is real (confirmed in `write.go`: `Content string` + `omitempty`, `sendContent = true` for create/update, and `schema.go` `required` = action, file_path only; `TestCommitFilesEmptyContentIsSent` locks in `""` but nothing covers an omitted key). It does not falsify SC3: the criterion asks that multi-file create/change/delete in one commit works, is visible in get_commit, and that the actions[] schema is accepted by the 1.30.x client - all true. SC4's error-message requirements concern create_or_update_file, whose `content` IS required in its inferred schema (no omitempty). Therefore CR-01 is recorded as a minor gap (`has_blocking_gaps: false`). It should nevertheless be fixed before the Phase 4 live run, because it is a silent destructive write on a tool annotated `DestructiveHint: true`, and the schema currently contradicts its own description.

WR-01, WR-02, WR-03 were re-read in code and confirmed; WR-04/WR-05 taken from the review (not re-executed). None breaks a success criterion.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/tools/branch.go` | list_branches, create_branch | VERIFIED | substantive, registered, GL client calls real |
| `internal/tools/commits.go`, `diff.go` | list_commits, get_commit with budgeted diff | VERIFIED | wired to Commits API, renderDiffFiles |
| `internal/tools/compare.go` | compare_refs | VERIFIED | own decode keeps collapsed/too_large flags |
| `internal/tools/write.go`, `schema.go` | commit_files, create_or_update_file | VERIFIED (CR-01) | shared commitCore, single POST |
| `internal/tools/errors.go` | write-aware wording | VERIFIED (WR-01) | rules present for all SC4 cases |
| `internal/tools/testdata/tools_list.golden.json` | golden of 12 tools | VERIFIED | 12 names: commit_files, compare_refs, create_branch, create_or_update_file, get_commit, get_file_contents, get_project, list_branches, list_commits, list_projects, list_repository_tree, whoami |
| `e2e/*`, `scripts/smoke.py` | real-binary + Python client proof | VERIFIED | e2e green; WR-04 caveat on smoke.py |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `register.go` | all 12 handlers | `mcp.AddTool(... safe(d, handler))` | WIRED |
| commit_files / create_or_update_file | GitLab POST /repository/commits | shared `commitCore` -> `Commits.CreateCommit` | WIRED (single call) |
| write failures | user text | `withWrite(op, ...)` -> `writeText` rules | WIRED |
| POST retry policy | glclient transport | `checkRetry` GET/HEAD only | WIRED |
| commit_files result | get_commit | full SHA in output + follow-up diff GET for +/- stats | WIRED |

### Data-Flow Trace (Level 4)

Tools return text built from live GitLab API responses (client-go typed calls / own decode of /compare); no hardcoded or static returns found. FLOWING.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Build and vet | `go build ./... ; go vet ./...` | clean | PASS |
| Full test suite (run once) | `go test ./...` | all packages ok (e2e, glclient, tools, config, logging) | PASS |
| Python mcp 1.30.0 client handshake + tools | `go test ./e2e -run Python -v` | `TestPythonSmoke` PASS (not SKIP) | PASS |
| Golden tools/list has 12 tools | grep of golden file | 12 tool names | PASS |

### Probe Execution

No `probe-*.sh` declared or present. SKIPPED.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| READ-05 | 02-01 | list_branches | SATISFIED | branch.go, registered, golden, tests |
| READ-06 | 02-02 | list_commits by ref/path | SATISFIED | commits.go Ref/Path options |
| READ-07 | 02-02 | get_commit with diff | SATISFIED | commits.go + diff.go |
| UTIL-02 | 02-03 | compare_refs | SATISFIED | compare.go |
| WRT-01 | 02-01 | create_branch from ref | SATISFIED | branch.go |
| WRT-02 | 02-04 | commit_files, actions[], no force | SATISFIED (CR-01 caveat) | write.go, schema.go; no `force` in options |
| WRT-03 | 02-05 | create_or_update_file | SATISFIED | write.go (shares commitCore with commit_files) |

All 7 phase requirement IDs appear in PLAN frontmatter (02-01..02-05) and in REQUIREMENTS.md; no orphaned Phase 2 requirements. Housekeeping: REQUIREMENTS.md still shows these 7 as unchecked / "Pending" in the traceability table - needs updating by the orchestrator when the phase is closed.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| internal/tools/write.go | ~59-64, ~130 | Presence of required field not enforced (CR-01) | Warning (minor gap) | silent empty-file commit |
| internal/tools/errors.go | writeText | Incomplete "may have been applied" kinds (WR-01) | Warning | possible duplicate write by model |
| internal/tools/compare.go | ~99-110 | Overflow on huge page (WR-03) | Warning | recovered panic, no crash |
| internal/tools/write.go | createOrUpdateFileDescription | Overstated concurrency protection (WR-02) | Warning | misleading tool description |

No TBD/FIXME/XXX markers found in internal/, e2e/, scripts/. Review INFO items (IN-01..IN-05) are non-blocking.

### Human Verification Required

None for this phase. Running the tools against real gitlab.com is deliberately deferred to Phase 4 and is not treated as a gap.

### Gaps Summary

The phase goal is achieved: all 12 tools are registered, the four ROADMAP success criteria are backed by real code, hermetic tests, the real binary, and the Python mcp 1.30.0 client. Remaining gaps are five minor write-safety/robustness items from the code review (CR-01 most important, then WR-01..WR-04). Recommended: park them to the backlog or fold a small fix-up (`/bm:code-review-fix`) in before Phase 4; CR-01 and WR-04 in particular should be fixed before the live write run against gitlab.com.

Note: ROADMAP marks the phase `mode: mvp` but its goal is not a user story; the mvp user-story guard was not applied, and verification used the ROADMAP success criteria.

---

_Verified: 2026-09-24_
_Verifier: Claude (gsd-verifier)_
