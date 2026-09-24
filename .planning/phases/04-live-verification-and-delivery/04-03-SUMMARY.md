---
phase: 04-live-verification-and-delivery
plan: 03
subsystem: delivery
tags: [go, readme, pe-imports, build-verification, docs]
requires: []
provides:
  - "TestBinaryHasNoExternalDependencies: hermetic canonical-build check (kernel32-only imports, CGO_ENABLED=0, -trimpath, module paths embedded, size < 25 MB)"
  - "README.md (Russian): build, PAT scopes, AiAdventAgentV2 connection (UI + REST), 20 tools, limitations, verification"
affects: [04-01, 04-02, 04-04]
tech-stack:
  added: []
  patterns: ["debug/pe ImportedSymbols check on a freshly built exe in t.TempDir()"]
key-files:
  created: [e2e/build_test.go, README.md]
  modified: []
key-decisions:
  - "Module presence asserted by path ('dep\\t<module>') only, versions stay in go.mod"
  - "README documents scripts/live.py CLI as specified in the plan (Plan 02 owns the script)"
requirements-completed: [QA-04]
duration: 15min
completed: 2026-09-25
---

# Phase 4 Plan 03: Delivery build check and README Summary

Hermetic test that the canonical release build is a single kernel32-only exe, plus a Russian README covering build, token scopes, agent connection, the 20 tools and honest limitations.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Hermetic no-external-dependencies build test | 997b4e7 |
| 2 | Russian README | 0bdebf4 |

## Verification (observed)

- `go test ./e2e -run TestBinaryHasNoExternalDependencies -count=1 -v`: PASS; log `size 11445760 bytes, 48 imported symbols`.
- `go test ./... -count=1`: all packages ok (e2e, config, glclient, logging, tools).
- README acceptance checks (run via Grep, because the compound bash verify loop was refused by the worktree sandbox): 20 tool table rows found; `$env:CGO_ENABLED = "0"`, `"args": []`, `scripts/live.py --project`, `C:\\Projects\\GitLabMcpMini\\gitlab-mcp.exe` present; 5 `glpat-` occurrences, all the `glpat-x{20}` placeholder; "Защищает от перезаписи" absent; `delete_branch` and "перевод строки" present.
- Not verified: `scripts/live.py` does not exist in this worktree (owned by Plan 02), so the README's live-run section documents its CLI from the plan spec and is unchecked against the real script. `smoke.py` flags were checked against the source.

## Deviations from Plan

None in the files touched. The full compound bash verify command was split into equivalent separate checks only because the sandbox rejected it.

## Issues / Notes for the orchestrator

- `internal/tools/write.go` `createOrUpdateFileDescription` still says "Защищает от перезаписи чужих правок: ... last_commit_id". This contradicts README limitation D-11 (no protection between the agent's own read and the write). It is outside this plan's files; another plan in the phase should correct the tool description.
- The README's exit-code meaning for live.py (1 = check failed, 2 = launch/config error) is inferred from the plan's "exit 0/1/2"; confirm against Plan 02's implementation.

## Known Stubs

None.

## Self-Check: PASSED

- e2e/build_test.go, README.md exist; commits 997b4e7 and 0bdebf4 exist.
