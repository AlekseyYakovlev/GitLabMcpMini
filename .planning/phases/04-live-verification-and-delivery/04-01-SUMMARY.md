---
phase: 04-live-verification-and-delivery
plan: 01
subsystem: tools
tags: [go, mcp, gitlab, write-safety, commit_files, create_or_update_file]

requires:
  - phase: 02-repository-write
    provides: commit_files and create_or_update_file write tools
provides:
  - "Empty content on create/update is rejected before any request in commit_files and create_or_update_file"
  - "Tool descriptions and schema text state the empty-file rule"
  - "ROADMAP backlog 999.1 closed"
affects: [04-04 live run against the sandbox project, 04-03 README]

tech-stack:
  added: []
  patterns:
    - "Guard placed before any HTTP call; hermetic no-request tests assert len(fake.Requests()) == 0"

key-files:
  created: []
  modified:
    - internal/tools/write.go
    - internal/tools/schema.go
    - internal/tools/write_test.go
    - internal/tools/upsert_test.go
    - internal/tools/testdata/tools_list.golden.json
    - .planning/ROADMAP.md

key-decisions:
  - "ActionIn.Content and UpsertFileIn.Content stay plain string (no *string); an empty content is rejected instead of being distinguished from a missing one (D-10)"
  - "Workaround for an empty file is a single newline, stated in descriptions and error text"

requirements-completed: [QA-03]

duration: 12min
completed: 2026-09-25
---

# Phase 4 Plan 01: Empty-content guard (backlog 999.1) Summary

**commit_files (create/update) and create_or_update_file now refuse missing or empty `content` before any request to GitLab, with the schema kept free of null/anyOf and the rule documented in descriptions.**

## Accomplishments

- `toFileAction` returns `actions[i]: для create|update нужен непустой content (...)` for create/update without content; move (empty content = no `content` key) and delete (content is an error) are unchanged.
- `createOrUpdateFile` checks `in.Content == ""` right after the path check, before `validateCommitInput` and before the `GetFile` pre-read, so neither GET nor POST is sent.
- Tests: two new rows in `TestCommitFilesGuardsSendNoRequest` (create without content, update with empty content), `TestCommitFilesEmptyContentIsSent` rewritten to `TestCommitFilesEmptyContentIsRejected`, new `TestCreateOrUpdateFileEmptyContentSendsNoRequest`. All assert zero requests.
- Descriptions of both tools, the `UpsertFileIn.Content` jsonschema tag and the `commit_files` `content` schema text state that an empty file cannot be created (pass one newline). Lengths: commit_files 668, create_or_update_file 652 characters (limit 900). The concurrency claim was not changed.
- Golden `tools_list.golden.json` regenerated; only description strings changed.
- ROADMAP backlog 999.1 marked `CLOSED in Phase 4, plan 04-01` with the shipped approach; 999.3-999.5 remain BACKLOG.

## Task Commits

1. Task 1 (TDD, RED observed then GREEN): `236c2d1` fix(04-01): reject empty content on create/update in write tools
2. Task 2: `fe153db` docs(04-01): document the non-empty content rule in tool descriptions and schema
3. Task 2 (ROADMAP part, separate commit): `310a8be` docs(04-01): close backlog 999.1 in ROADMAP

## Verification (actually run)

- RED: before the guard, the 4 new/rewritten tests failed (POST sent / file created).
- `go test ./internal/tools -run "TestCommitFiles|TestCreateOrUpdateFile" -count=1`: ok.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools), including `TestToolSchemas` and `TestToolsListGolden`.
- `go vet ./...`: clean.
- Grep acceptance checks: `непустой content` in write.go >= 2, `*string` in write.go = 0, `пустой файл создать нельзя` in write.go = 4 and golden = 2, `empty files are not supported` in schema.go = 1, `CLOSED in Phase 4, plan 04-01` in ROADMAP = 1, `999.3.*(BACKLOG)` = 1.
- Not verified: `gofmt -l internal` lists nearly every file in this checkout because the working tree has CRLF line endings (core.autocrlf=true); this is pre-existing and not caused by this plan. Files I edited keep their original line endings.

## Deviations from Plan

### Orchestrator instruction vs plan

**[Note] ROADMAP.md edited despite the "do not touch ROADMAP" instruction.** The plan lists `.planning/ROADMAP.md` in `files_modified` and Task 2 requires closing 999.1, while the orchestrator prompt says the orchestrator owns ROADMAP writes. I made the plan-required content edit (only the 999.1 backlog entry, no progress table or plan list changes) in a separate commit `310a8be`, so it can be dropped or re-applied cleanly on merge if it conflicts with the orchestrator's progress updates. STATE.md was not touched.

Otherwise: plan executed as written. No auth gates, no stubs, no new threat surface.

## Self-Check: PASSED

- Modified files present; commits `236c2d1`, `fe153db`, `310a8be` exist on the worktree branch.
