---
phase: 04-live-verification-and-delivery
plan: 02
subsystem: testing
tags: [python, mcp-1.30.0, stdio, gitlab-com, live-run, transcript, rest-cleanup]

requires:
  - phase: 03-write-tools
    provides: 20-tool server with Russian error wording (errors.go, mr_status.go, mr.go)
provides:
  - scripts/live.py: opt-in live scenario against real gitlab.com with guarded writes, redacted transcript and REST cleanup
  - e2e/live_script_test.go: hermetic proof of the opt-in gate (exit 2, no transcript, --help)
affects: [04-04 live run, README (04-03) CLI contract]

tech-stack:
  added: []
  patterns:
    - "one MCP session per token with explicit env (parent env is not inherited by stdio_client)"
    - "write-target guard in the call helper, raised before the request is sent"
    - "cleanup outside the server via urllib + PRIVATE-TOKEN, tracked names only"

key-files:
  created: [scripts/live.py, e2e/live_script_test.go]
  modified: []

key-decisions:
  - "Merge-of-merged-MR check accepts 'MR уже влит', 'not_open' or 'MR не открыт' because the server answers with stateAdvice text for merged MRs"
  - "Error-block steps are non-fatal so one wrong wording does not hide the rest; the main chain stays fatal"
  - "Any SKIPPED step other than the read-only one counts as a failure (exit 1, criterion FAIL)"
  - "Token leak into a tool response fails the step (raw text checked against all known secrets before redaction)"

patterns-established:
  - "Step recorder with PASS/FAIL/SKIPPED and criterion tags (1 main, 2 errors) feeding the transcript criteria table"
  - "Transcript written in finally after cleanup; asserts no secret, no glpat- string, no whoami identity line"

requirements-completed: []  # QA-03 tooling delivered here; the requirement closes only after the real run in plan 04

duration: 40min
completed: 2026-09-25
---

# Phase 4 Plan 02: live.py opt-in live run Summary

**Opt-in `scripts/live.py` drives the real `gitlab-mcp.exe` over mcp==1.30.0 stdio through read, branch, commit, MR, note, merge and real error probes on gitlab.com, cleans up its own objects via REST, and writes a redacted LIVE-RUN.md; proven hermetically only (no real run, no tokens).**

## Performance

- **Duration:** about 40 min
- **Completed:** 2026-09-25
- **Tasks:** 3 of 3
- **Files created:** 2 (scripts/live.py, e2e/live_script_test.go)

## Accomplishments

- CLI contract as planned: `--project` required with no default, `--exe`, `--out`; exit 2 (missing project, missing GITLAB_TOKEN, missing exe) happens before any network call or file write; `GITLAB_URL` is never read or passed.
- Preflight recorded in the transcript header: exe size, SHA-256, staleness warning, `go version -m` excerpt, mcp client version, negotiated protocolVersion.
- Main scenario (steps 0-18) with value chaining (default branch, commit SHA, MR iid), polling of `detailed_merge_status`, merge with `remove_source_branch`, post-merge verification and a warn-not-fail poll for asynchronous branch removal.
- Error block E1-E10 plus session B (invalid dummy token, 401 without echo) and session C (read_api token: scopes parsed as list elements, single create_branch write proof expecting `403: запись отклонена` with scope `api`; explicit SKIPPED when GITLAB_TOKEN_READONLY is unset, criterion 2 PARTIAL).
- `check_write_target` guards `branch` (all write tools) and `source_branch` (create/update MR) against the default branch and non `live/<ts>-` names; target_branch == default is intentionally allowed.
- REST cleanup in `finally`: closes open MRs of tracked branches, deletes only tracked `live/<ts>-*` branches (204/404 ok), per-object error isolation, `cleanup INCOMPLETE:` entries force exit 1.
- Transcript writer: full redaction of main, read-only and dummy tokens, identity redaction of the whoami `@user (name), id N` line (scopes line kept), assertions before writing.
- `scripts/smoke.py` untouched.

## Task Commits

1. **Tasks 1-3 (script)** - `130fd95` feat(04-02): add opt-in live.py run against gitlab.com. The three script tasks all edit the single file `scripts/live.py`; it was written in one pass, so they share one commit.
2. **Task 3 (test)** - `9281371` test(04-02): add hermetic usage test for live.py

## Verification actually run

- `uv run --with mcp==1.30.0 python -m py_compile scripts/live.py` : OK.
- `uv run scripts/live.py --help` : exit 0, lists `--project`, `--exe`, `--out`.
- `uv run scripts/live.py` (no args): exit 2. With `--project g/p` and no GITLAB_TOKEN: exit 2, stderr names GITLAB_TOKEN, no transcript file created.
- `go test ./e2e -run TestLiveScriptUsage -count=1 -v`: PASS (3 subtests). `go test ./... -count=1`: all packages ok. `go vet ./...`: clean.
- Ad-hoc self-check (scratch, not committed) of internals: write-target guard blocks `branch=main`, non-`live/<ts>-` names and `source_branch=main`, allows `live/<ts>-*` and `target_branch=main`; `redact`, `redact_identity`, `parse_scopes`; `write_transcript` produced a document without the secret and without the identity line.
- Ad-hoc check against a freshly built exe: `open_session` initialize, `tools/list` returned 20 tools, protocolVersion 2025-11-25; session B with a dummy token got the real gitlab.com 401 (`401: GitLab отклонил токен ...`) with the dummy absent; session C produced the explicit SKIPPED with no token.
- `grep -c GITLAB_URL scripts/live.py` returns 0; `scripts/smoke.py` has no diff.

**Not verified (needs tokens and gitlab.com, plan 04):** the main scenario against real gitlab.com (steps 1-18), error block E1-E10 wording, the read-only 403 wording, REST cleanup calls, and a full transcript from a real run. The main chain, the error block and the cleanup were checked only by code reading and syntax compile, never executed end to end.

## Deviations from Plan

**1. [Rule 1 - Bug] E10 expected wording corrected to the real server text**
- **Found during:** Task 3 (reading internal/tools/mr_status.go and mr_merge.go)
- **Issue:** The plan expects `not_open` or `MR не открыт`, but `merge_merge_request` on a merged MR returns `stateAdvice` text `MR уже влит (state=merged)` before it ever looks at `detailed_merge_status`. The plan's substrings would never match.
- **Fix:** E10 accepts any of `MR уже влит`, `not_open`, `MR не открыт`.
- **Files modified:** scripts/live.py

**2. [Process] Tasks 1-3 committed as two commits instead of three**
- The three script tasks all edit scripts/live.py, which was authored in one pass; splitting retroactively would have produced intermediate commits that were never verified as such. Script and test are separate commits.

## Assumption Drift (advisory)

- **Found during:** Task 3. **Planned:** merge of a merged MR reports `not_open`/`MR не открыт`. **Actual:** `MR уже влит (state=merged)`. **Why:** state is checked before merge status in mergeMergeRequest.

## Known Stubs

None.

## Threat Flags

None. The script adds no server surface; new outbound surface (REST DELETE/PUT with PRIVATE-TOKEN) is the planned cleanup, covered by T-04-08.

## Notes for plan 04 and README

- The `commit_files` create-without-content step (E3) expects `непустой content`, which is produced by the 999.1 fix from plan 01; the exe must be rebuilt after plan 01 is merged (the script warns when the exe is older than the sources).
- Exit code semantics: 0 all PASS (read-only step may be SKIPPED), 1 any FAIL, unexpected SKIPPED or cleanup INCOMPLETE, 2 usage/env.

## Self-Check: PASSED

- scripts/live.py, e2e/live_script_test.go exist; commits 130fd95 and 9281371 exist on the worktree branch.
