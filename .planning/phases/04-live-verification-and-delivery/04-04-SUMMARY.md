---
phase: 04-live-verification-and-delivery
plan: 04
subsystem: testing
tags: [live-verification, gitlab.com, evidence, build, sandbox]
requires:
  - phase: 04-live-verification-and-delivery
    provides: "999.1 fix (04-01), scripts/live.py (04-02), README (04-03)"
provides:
  - "Green live run of the full scenario on gitlab.com (QA-03) committed as LIVE-RUN.md"
  - "Canonical CGO_ENABLED=0 gitlab-mcp.exe with recorded size and SHA-256 (QA-04 build proof)"
affects: []
tech-stack:
  added: []
  patterns: ["live evidence transcript with header hash equal to the built exe"]
key-files:
  created:
    - .planning/phases/04-live-verification-and-delivery/LIVE-RUN.md
  modified: []
key-decisions:
  - "No D-04 fixes were needed: the live run was green on the first attempt, so no production or script code changed in this plan."
patterns-established: []
requirements-completed: [QA-03, QA-04]
duration: n/a
completed: 2026-09-25
---

# Phase 4 Plan 04: Live run on gitlab.com Summary

**Full read -> branch -> commit -> MR -> note -> merge -> cleanup scenario plus real error cases ran green against AlekseyYakovlev/sanbox with a freshly rebuilt, dependency-free 11.4 MB exe; transcript committed as LIVE-RUN.md.**

## Accomplishments

- Rebuilt `gitlab-mcp.exe` canonically (`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`, go1.27.1): 11446784 bytes, SHA-256 `955f26d02ebd808d3126d41ad7b159d29113695d5eb82171a681a4d9f6853a71`. The exe is gitignored and not committed. `go vet ./...` and `go test ./... -count=1` were green (re-run at the end of Task 3 as well: e2e, config, glclient, logging, tools all ok).
- The author ran `uv run scripts/live.py --project AlekseyYakovlev/sanbox` in their own PowerShell (Option A, run label 20260925-094734), so no token entered the agent's environment. Result: every step PASS, criterion 1 PASS, criterion 2 PASS, cleanup PASS, final line "LIVE OK".
- Covered live: tools/list (20 tools), whoami (scopes [api]), get_project, tree, file read, create_branch, commit_files (3 creates), create_or_update_file (update and create), get_commit, compare_refs, create_merge_request, note create/list, MR diffs, mergeable wait, merge, post-merge checks; errors: duplicate branch (400 "ветка уже существует"), empty content, Draft MR merge refused (draft_status), duplicate MR ("открытый MR уже есть: !2"), 404 for file, ref and project, merge of already-merged MR, invalid token (401, token not echoed), read-only token (whoami scopes [read_api], reads OK, create_branch -> 403 with the `api` scope hint).
- LIVE-RUN.md verified on disk before commit: header date 2026-09-25T09:47:34Z and label 20260925-094734; header SHA-256 equals `sha256sum gitlab-mcp.exe`; `go version -m` excerpt contains `CGO_ENABLED=0` and `-trimpath=true`; 43 PASS, zero FAIL, no "cleanup INCOMPLETE"; contains "cleanup (вне сервера)"; `glpat-` regex count 0 and no other token-like strings (the only long strings are commit SHAs, module hashes and the sandbox bot username); the plan's automated verify command printed GREEN.
- Evidence commit `b4fd7be` touches only LIVE-RUN.md; `git ls-files .planning/.pending-auth-captures.jsonl` prints nothing (never staged, neither was `.planning/config.json`).

## Task Commits

1. Task 1: Rebuild exe + hermetic gate - no commit (build output is gitignored)
2. Task 2: Author live run (human-action checkpoint, resolved via Option A) - no commit (author's run wrote LIVE-RUN.md)
3. Task 3: Evaluate and commit evidence - `b4fd7be` docs(04): live run evidence on gitlab.com (QA-03)

## Files Created/Modified

- `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md` - redacted transcript of the green live run

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking/Note] `gofmt -l` lists files because of CRLF working-tree endings**
- **Found during:** Task 1
- **Issue:** The plan requires `gofmt -l cmd internal e2e` to print nothing, but it lists 55 files: the working tree has CRLF line endings (`core.autocrlf=true`) while the index holds LF.
- **Fix:** No code change. With CRs stripped, every tracked `*.go` file is gofmt-clean, so this is an environment artifact, not a formatting defect.
- **Files modified:** none

No D-04 fixes were required (no server bug, no live.py false failure, no sandbox misconfiguration).

## Observations

- **Step 18 WARN (non-fatal):** after merging with `remove_source_branch`, `list_branches` printed "WARN: ветка ещё не удалена (удаление асинхронное)" in the script note while the tool itself returned "ветки не найдены"; the step is PASS (source branch deletion on GitLab is asynchronous and cleanup handles leftovers).
- **Sandbox residue:** MR !1 (merged) and MR !2 (closed) stay in AlekseyYakovlev/sanbox as history, together with merged commits under `live-run/20260925-094734/` on `main`. Branches: `live/20260925-094734-main` and `-ro` already absent, `-draft` deleted by cleanup. No live branch and no open MR remain.

## Known Stubs

None.

## Threat Flags

None (no new code or trust-boundary surface in this plan).

## Reminder for the author

The PATs used were pasted into a chat during phase discussion. Revoke and reissue both `GITLAB_TOKEN` (api) and `GITLAB_TOKEN_READONLY` (read_api) now that Phase 4 is done.

## Verification limits

The live steps were executed by the author, not by the executor; the executor verified the resulting LIVE-RUN.md on disk (hash, flags, redaction, PASS/FAIL counts) and re-ran the hermetic suite, but did not independently re-query gitlab.com.

## Self-Check: PASSED

- LIVE-RUN.md present; commit b4fd7be exists and lists only that file.
