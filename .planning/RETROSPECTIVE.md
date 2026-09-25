# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-09-25 (post-ship quick task 260926-24l on 2026-09-26)
**Phases:** 4 | **Plans:** 20 | **Sessions:** not tracked

### What Was Built
- Go stdio MCP server (`gitlab-mcp.exe`, no runtime dependencies) with 20 tools: projects, tree/files, branches, commits, compare, MRs (read, create, note, update, merge) and repository writes.
- Error layer that maps every GitLab failure to a short Russian message, with write-specific "outcome unknown" wording and no retries of writes.
- Test harness: fake GitLab over httptest, in-memory MCP sessions, real-binary e2e and a Python `mcp==1.30.0` smoke client; opt-in `scripts/live.py` scenario against gitlab.com.

### What Worked
- The up-front spike (Go server driven by the agent's own `mcp` 1.30.0 client) removed the language/SDK risk before any tool was written.
- Vertical slices with shared helpers built in Phase 1 (budget, error mapper, session helper) kept later tools small.
- Hermetic tests plus one live run caught different things; the live run is what surfaced gitlab.com specifics.

### What Was Inefficient
- Review findings from Phase 2 (CR-01, WR-01..04) sat as verification "gaps" until the milestone close and had to be reconciled by hand.
- `milestone.complete` counted backlog `999.x` directories as phases (9 instead of 4); MILESTONES.md was corrected manually.

### Patterns Established
- Tool errors are wrapped with `withWrite`/`withProject`; anything that needs extra GitLab context to word an error is resolved in `safe`, not in each handler.
- Tool descriptions are pinned by a golden `tools/list` file; behaviour changes regenerate it with `-update`.

### Key Lessons
1. gitlab.com specifics only show up live: `order_by=last_activity_at` answered 500 on some tokens (fixed in 2af4fdd), and a generic 403 hid that a project was scheduled for deletion (quick 260926-24l).
2. `simple=true` on the projects list drops `archived` and `marked_for_deletion_on`; check which fields a projection returns before relying on them.
3. A token in the environment may not be the token the agent uses; role-dependent paths (Reporter) need unit tests, not just a live check.

### Cost Observations
- Model mix and session counts were not recorded for this milestone.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | n/a | 4 | Vertical MVP with a live verification phase at the end |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | `go test ./...` green (count not tracked) | not measured | 0 (go-sdk and client-go only) |

### Top Lessons (Verified Across Milestones)

1. Verify against the real service early; hermetic tests alone missed gitlab.com behaviour.
