# Feature Research

**Domain:** GitLab MCP server (stdio, gitlab.com, PAT auth), scoped to code reading, Merge Requests and repository writes
**Researched:** 2026-09-24
**Confidence:** MEDIUM-HIGH (official tool list fetched from GitLab docs; community lists from READMEs; a few REST API details flagged LOW where not verified)

## Reference Points

| Server | Status | Tool count | Auth / transport | Relevance |
|--------|--------|-----------|------------------|-----------|
| **modelcontextprotocol/servers `gitlab`** | Archived 2025-05-29, read-only repo | 9 | PAT, stdio | The "classic" minimal set. Gaps: no MR list/diff/comments/merge, no tree/commits/branches list/search code. Useful as a naming baseline, not a feature target. |
| **Official GitLab MCP server** (`/api/v4/mcp`, Beta since 18.6) | Active, Free/Premium/Ultimate | ~50 (grows every release, mostly 19.x) | OAuth 2.0 DCR, HTTP; stdio only via `mcp-remote` (Node) | Best current reference for tool *shape*. No PAT documented on the fetched page, which is exactly why a local PAT/stdio server has a reason to exist. |
| **zereight/gitlab-mcp** (`@zereight/mcp-gitlab`) | Active, most popular community server | "261 tools + `discover_tools`" | PAT / OAuth2, stdio + SSE + HTTP | Shows the sprawl to avoid, but also the good ideas: permission modes, toolset filtering, 2-step MR review, JMESPath filter. |
| Small MR-focused servers (e.g. kopfrechner/gitlab-mr-mcp) | Active | ~10 | PAT, stdio | Confirms a "get MR details + comments + diff + add comment" core is what people actually want. |

The official server's in-scope tools (our reference for shape): `get_project`, `list_projects`, `list_branches`, `add_branch`, `list_repository_tree`, `get_repository_file` (offset/limit), `list_commits`, `get_commit` (optional diff/notes), `add_commit` (actions array), `search`, `list_merge_requests`, `get_merge_request` (with `include` for diffs/commits/notes/pipelines/discussions/conflicts), `get_merge_request_diffs`, `get_merge_request_commits`, `get_merge_request_notes`, `get_merge_request_conflicts`, `save_merge_request` (create-or-update), `save_note`, `save_merge_request_review`, `accept_merge_request`. Everything else (issues/work items, pipelines, Duo sessions, vulnerabilities, wiki, releases) is out of our scope.

## Feature Landscape

### Table Stakes (Users Expect These)

Any GitLab MCP that lacks these is not useful to an agent. Every one maps to an Active requirement in PROJECT.md.

**Code reading**

| Feature (tool) | Why Expected | Complexity | Notes |
|----------------|--------------|------------|-------|
| `list_projects` / search projects | Agent must discover project IDs/paths; present in every server (`search_repositories` in the archived one) | LOW | `GET /projects` with `search`, `membership=true`, `owned`, pagination. Default to `membership=true` so results are the user's, not all of gitlab.com. |
| `get_project` | Resolve default branch, visibility, numeric ID from a path; needed before most other calls | LOW | Accept `group/sub/proj` path and numeric ID; URL-encode the path (`%2F`). Official server added it late (19.4), which shows agents needed it. |
| `list_repository_tree` | Explore a repo before reading files | LOW | `GET /repository/tree` with `path`, `ref`, `recursive`, pagination. Cap recursive output. |
| `get_file_contents` | The single most used code-reading tool | LOW-MED | API returns base64 in JSON; decode to text. Handle binary/large files (return metadata + "binary/too large" message instead of garbage). Require/default `ref`. Official offers `offset`/`limit` for huge files. |
| `list_branches` (with `search`) | Needed to pick refs; requested explicitly | LOW | Pagination. |
| `list_commits` | History browsing; requested explicitly | LOW | Filters: `ref_name`, `path`, `author`, `since`/`until`. |
| `get_commit` (with diff) | "What did this commit change?" is the next question after `list_commits` | LOW-MED | Commit + `/diff` endpoint; cap diff size. |
| `search_code` | Explicit requirement; agents grep before they read | MED | `GET /projects/:id/search?scope=blobs`. Project-scoped is reliable; global/group blob search on gitlab.com depends on search backend availability (LOW confidence, verify against real gitlab.com). Results are fragments with line numbers, not full files. |

**Merge Requests**

| Feature (tool) | Why Expected | Complexity | Notes |
|----------------|--------------|------------|-------|
| `list_merge_requests` | Entry point to all MR work | LOW | Project-scoped and optionally global (`scope=assigned_to_me` / `created_by_me`), filters `state`, `author`, `reviewer`, `search`, `target_branch`. Default `state=opened`. |
| `get_merge_request` | Details: title, description, branches, `detailed_merge_status`, `diff_refs`, author | LOW | Use `detailed_merge_status` (the old `merge_status` is deprecated since 15.6). `diff_refs` may be empty right after creation (populated async). |
| `get_merge_request_diffs` | The core of code review by an agent | MED | Paginate; per-file diffs include `too_large`/`collapsed`/`generated` style flags, and `changes_count` is capped at "1000+". Surface truncation explicitly to the model. Prefer the paginated `/diffs` endpoint over the deprecated `/changes` (MEDIUM confidence, verify on gitlab.com). |
| `create_merge_request` | Explicit requirement; present even in the archived server | LOW-MED | `source_branch`, `target_branch`, `title`, `description`, `remove_source_branch`, `squash`. Needs project ID + existing branches. |
| `create_merge_request_note` (comment) | Explicit requirement; every MR-capable server has it | LOW | General (non-inline) note via `POST /notes`. Optionally `discussion_id` to reply in a thread. |
| `list_merge_request_notes` / discussions | A reviewer agent must read existing comments before adding one; official has `get_merge_request_notes` | LOW-MED | Return note author, body, resolved status, and system-note flag so the model can skip "assigned to X" noise. Paginate. |
| `merge_merge_request` | Explicit requirement (official: `accept_merge_request`) | MED | `PUT /merge`. Must handle 405/406/409 (not mergeable, conflicts, pipeline not passed, SHA mismatch) with readable errors. Support `squash`, `should_remove_source_branch`, `merge_commit_message`, and an optional `sha` guard. |

**Repository writes**

| Feature (tool) | Why Expected | Complexity | Notes |
|----------------|--------------|------------|-------|
| `create_branch` | Explicit requirement; in archived server and official (`add_branch`) | LOW | `POST /repository/branches` with `branch` + `ref`. Prerequisite for any MR flow. |
| `create_or_update_file` (single file) | Explicit requirement; the archived server's core write tool | MED | Create vs update use different HTTP verbs (POST vs PUT) on the files API; update needs correct existing state. Provide `branch`, `commit_message`, optional `start_branch`. Either expose explicit `create`/`update` or auto-detect via a HEAD/GET first. Content encoding (`text` vs `base64`) must be handled. |
| `commit_files` / `push_files` (multi-file commit) | Explicit requirement ("коммиты"); present in archived (`push_files`) and official (`add_commit`) | MED | `POST /repository/commits` with `actions[]` (`create`, `update`, `delete`, `move`, `chmod`), `start_branch`, atomic. This is the better primitive; single-file tool can be a thin wrapper over it. |

**Cross-cutting (users notice when absent)**

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Project identified by numeric ID **or** `namespace/path` | Every server does this; agents only know paths | LOW | One shared resolver + URL-encoding helper used by all tools. |
| Pagination parameters (`page`, `per_page`) and "has more" hint in output | List tools without it silently truncate | LOW | Cap `per_page` (GitLab max 100). Say "more results available, use page=N". |
| Readable errors (401/403/404/409/422/429, network, timeout) returned as tool errors (`isError`) rather than crashes | Explicit requirement; #1 complaint category with community servers | MED | Map status codes to messages with actionable hints (e.g. 404 on private project = token lacks access). Honor `Retry-After` on 429. |
| Output size limiting | Diffs, files and trees can blow the model context | MED | Hard character cap with explicit "[truncated: N of M]" marker. |
| Accurate `inputSchema` with descriptions, required fields, enums | Client is MCP SDK 1.30.x; sloppy schemas make models call tools wrongly | LOW | Descriptions are the "prompt" of the tool; write them deliberately. |
| stdlib-level MCP lifecycle correctness (initialize, tools/list, tools/call, no stdout pollution, logs to stderr) | Stdio corrupts if anything else prints to stdout | LOW | Core Value in PROJECT.md. |

### Differentiators (Competitive Advantage)

Not needed for the tool to be usable. Chosen for fit with Core Value ("small, understandable, works on real gitlab.com"), not for feature parity.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Accept a GitLab **web URL** in place of project/MR IDs | Official server does it (`url` param). Agents and users paste URLs; removes ID-lookup round trips | LOW-MED | Parse `https://gitlab.com/group/proj/-/merge_requests/12`. Nice for manual testing too. |
| Inline (line-level) MR comments via discussion `position` | Real code-review capability, not just a general comment; official has it inside `save_merge_request_review` | HIGH | Requires `diff_refs` (`base_sha`, `start_sha`, `head_sha`), `old_path`/`new_path`, `old_line`/`new_line`. Easy to get 400s. Ship after general notes work. |
| `update_merge_request` (title/description/labels/close/reopen) | Completes MR lifecycle; official folds it into `save_merge_request` | LOW | Small; likely first candidate to add after MVP. |
| `approve_merge_request` | Common in community servers | LOW | Approvals may be Premium-gated on some rules (verify). Low value for a single-user learning project. |
| `compare_refs` / `get_branch_diffs` | "What would this branch change vs main?" before opening an MR. In zereight as `get_branch_diffs` | LOW-MED | `GET /repository/compare?from=&to=`. Cheap and useful with the write flow. |
| `get_merge_request_commits` | Official has it; helps summarize an MR | LOW | Can be an `include` option of `get_merge_request` instead. |
| Two-step MR review (list changed files first, then per-file diff) | zereight's agent-friendly design; avoids giant single-diff responses | MED | Alternative to pagination: `list_merge_request_changed_files` + `get_merge_request_file_diff`. Pick this or plain pagination, not both. |
| `merge` safety inputs: `sha` guard, merge-when-pipeline-succeeds | Prevents merging stale code; official exposes `sha` and `strategy` | LOW-MED | Cheap to add to `merge_merge_request`; pipeline-related option is only a flag, no pipeline tools needed. |
| Line-range read for files (`start_line`/`end_line` or `offset`/`limit`) | Lets model read part of huge files | LOW-MED | GitLab has a `/raw` endpoint; slicing is done locally. Official has `offset`/`limit`. |
| MCP tool annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`) | Lets clients label safe vs mutating tools without us adding a read-only mode | LOW | Metadata only; does not conflict with the "no write restrictions" decision. Verify the agent's SDK 1.30.x passes annotations through. |
| Compact, model-oriented responses (trim GitLab JSON to useful fields) | Fewer tokens, fewer hallucinations; a stated pain in large servers | LOW-MED | Whitelist fields per tool. This is the cheapest "quality" differentiator. |
| Structured output (`outputSchema`/`structuredContent`) alongside text | Modern MCP practice | MED | Only if the client SDK handles it; text output is sufficient for this agent. |
| `get_mcp_server_version` / health-ish tool | Official has it; trivial smoke test on the real stdio path | LOW | Useful for the manual verification requirement, `whoami`-style (`GET /user`) is even better for validating the PAT. |
| `delete_file` (via `commit_files` action) and `delete_branch` | Completes write CRUD | LOW | Delete-file already comes free via `actions[]`. `delete_branch` is destructive and not listed in Active requirements: defer. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Issues / work items, labels, milestones tools | Present in every other server; `create_issue` is even in the archived set | Explicitly out of scope ("Mini"); work items alone are ~10 tools in the official server | Ship none. Note MR `labels` parameter is a plain string array, no label tooling needed. |
| Pipelines / jobs / CI logs / artifacts | Agents love "why did CI fail" | Out of scope; large, log-heavy outputs. Note official `get_merge_request_pipelines` exists but is not needed | Only pass through `detailed_merge_status` / `head_pipeline` summary fields already in MR JSON. |
| Wiki, releases, tags, webhooks, variables, groups admin, vulnerabilities | Breadth in zereight (261 tools) | Scope creep; each area adds schema, tests, errors | Out of Scope per PROJECT.md. |
| `create_repository` / `fork_repository` | In the archived server | Project-level creation, not in requirement list; forks are async and need namespace handling | Skip. Revisit only if a concrete need appears. |
| 200+ granular tools or dynamic `discover_tools` | Coverage | Bloats `tools/list`, confuses tool selection, dynamic listing needs `listChanged` notifications | Keep to roughly 18-22 tools. Small list is the product. |
| OAuth (DCR, browser flow), remote/multi-user auth, HTTP/SSE transports | Official and zereight support them | Already Out of Scope; stdio agent only; PAT suffices | PAT from `GITLAB_TOKEN`. |
| Read-only / permission mode, confirmation prompts on writes | Common safety feature (zereight `GITLAB_PERMISSION_MODE`) | Author explicitly decided against; would add config surface | Rely on token scope (`read_api` vs `api`). Use tool annotations (see differentiators) for metadata only. |
| Self-hosted GitLab support/testing | Popular for community servers | Not verifiable by author; URL may be a config value but untested | `GITLAB_URL` default `https://gitlab.com`, undocumented as supported. |
| Semantic search / Duo integration | Official has `semantic_search`, Duo session tools | Depends on paid/beta features and instance config | `search_code` via basic blob search only. |
| Duplicate or "superseded" tool variants (`get_issue` + `get_work_item`) | Legacy compatibility | Confuses model tool choice | One tool per job. |
| Returning raw GitLab JSON dumps | Zero-effort implementation | Token waste, noise, secrets/URLs the model does not need | Field whitelist + truncation markers. |
| Silent truncation / silent pagination | Simpler code | Model reasons over incomplete data and does not know | Always print counts and "truncated"/"more pages" messages. |
| Bulk destructive actions (delete project, force push, delete branches en masse) | "Complete" write API | Unrestricted write policy means one bad model call is irreversible | Do not implement destructive tools beyond what the Active list needs. |

## Feature Dependencies

```
Project resolver (ID | path | URL) + HTTP client + error mapping + pagination helper
    └──required by──> every tool below

list_projects ──> get_project ──> list_branches / list_repository_tree / list_commits
list_repository_tree ──enhances──> get_file_contents
search_code ──enhances──> get_file_contents

create_branch ──requires──> (project + ref from list_branches / get_project default_branch)
commit_files (multi-file) ──requires──> branch exists (or start_branch)
create_or_update_file ──implemented via / parallels──> commit_files
create_merge_request ──requires──> source branch with commits ahead of target
                                   (create_branch -> commit_files/create_or_update_file -> create_merge_request)

list_merge_requests ──> get_merge_request ──> get_merge_request_diffs
                                         └──> list_merge_request_notes ──> create_merge_request_note
                                                                    └──> inline comment (needs diff_refs from get_merge_request)
get_merge_request ──requires──> merge_merge_request (check detailed_merge_status first)

Output truncation helper ──required by──> get_file_contents, get_merge_request_diffs, get_commit, search_code
```

### Dependency Notes

- **Shared foundation first:** project-ID resolution, authenticated HTTP client, error-to-message mapping, pagination and truncation helpers are prerequisites of every tool. Build and test them (against a mock and real gitlab.com) before adding tools.
- **Write flow is a chain:** `create_branch` -> commit -> `create_merge_request` -> comment -> `merge`. An end-to-end test of this chain on a throwaway gitlab.com project is the best "done" check for the "auto tests + manual stdio check" criterion.
- **`create_or_update_file` vs `commit_files`:** implement `commit_files` (actions array) as the primitive; single-file tool is a convenience wrapper. Both are in scope; do not build two independent code paths.
- **Inline comments require `get_merge_request`'s `diff_refs`**, and general notes do not. Hence inline comments are a later differentiator.
- **`merge_merge_request` depends on reading MR state** (`detailed_merge_status`, `sha`) to give useful errors; ship it after `get_merge_request`.
- **`search_code` conflicts with nothing** but depends on GitLab search backend behavior on gitlab.com; verify early on the real service.

## MVP Definition

### Launch With (v1)

Matches PROJECT.md Active requirements. Roughly 18 tools.

- [ ] Foundation: stdio lifecycle, PAT auth via `GITLAB_TOKEN`, project resolver, error mapping, pagination + truncation helpers
- [ ] Read: `list_projects`, `get_project`, `list_repository_tree`, `get_file_contents`, `list_branches`, `list_commits`, `get_commit`, `search_code`
- [ ] MR: `list_merge_requests`, `get_merge_request`, `get_merge_request_diffs`, `list_merge_request_notes`, `create_merge_request`, `create_merge_request_note`, `merge_merge_request`
- [ ] Write: `create_branch`, `commit_files` (multi-file), `create_or_update_file`
- [ ] Verification: tests with mocked HTTP + scripted stdio client run against real gitlab.com (including the create-branch -> commit -> MR -> comment -> merge chain on a scratch project)

### Add After Validation (v1.x)

- [ ] `update_merge_request` (edit/close) — completes MR lifecycle; trivial after `create_merge_request`
- [ ] `compare_refs` — trigger: wanting a diff before opening an MR
- [ ] URL-as-input for MR/project/file tools — trigger: friction pasting GitLab URLs in the agent
- [ ] Tool annotations (`readOnlyHint`/`destructiveHint`) — trigger: agent client shows/uses them
- [ ] `whoami` (`GET /user`) — trigger: debugging tokens/scopes
- [ ] `sha` guard and squash options for merge, `include` options on `get_merge_request`

### Future Consideration (v2+)

- [ ] Inline diff comments with positions — high complexity, needs `diff_refs` handling and thorough real-service tests
- [ ] `approve_merge_request` — only if review-flow experiments need it
- [ ] `delete_branch`, tags — destructive/out of scope until a use appears
- [ ] Structured outputs / `outputSchema` — only if the client benefits
- [ ] Two-step per-file MR diff tools — only if pagination proves insufficient on big MRs

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Foundation (resolver, HTTP, errors, pagination, truncation) | HIGH | MEDIUM | P1 |
| `get_file_contents` | HIGH | LOW-MED | P1 |
| `list_repository_tree` | HIGH | LOW | P1 |
| `list_projects` / `get_project` | HIGH | LOW | P1 |
| `list_branches` / `list_commits` / `get_commit` | HIGH | LOW | P1 |
| `search_code` | HIGH | MEDIUM | P1 |
| `list_merge_requests` / `get_merge_request` | HIGH | LOW | P1 |
| `get_merge_request_diffs` | HIGH | MEDIUM | P1 |
| `list_merge_request_notes` | MEDIUM-HIGH | LOW-MED | P1 |
| `create_merge_request` | HIGH | LOW-MED | P1 |
| `create_merge_request_note` | HIGH | LOW | P1 |
| `merge_merge_request` | HIGH | MEDIUM | P1 |
| `create_branch` | HIGH | LOW | P1 |
| `commit_files` (multi-file) | HIGH | MEDIUM | P1 |
| `create_or_update_file` | HIGH | MEDIUM | P1 |
| `update_merge_request` | MEDIUM | LOW | P2 |
| `compare_refs` | MEDIUM | LOW-MED | P2 |
| URL as input | MEDIUM | LOW-MED | P2 |
| Tool annotations | MEDIUM | LOW | P2 |
| `whoami` | MEDIUM | LOW | P2 |
| Compact field-whitelisted responses | MEDIUM-HIGH | LOW-MED | P2 (do it from the start where cheap) |
| Inline diff comments | MEDIUM | HIGH | P3 |
| `approve_merge_request` | LOW | LOW | P3 |
| Structured output | LOW | MEDIUM | P3 |
| Anything in Anti-Features | — | — | Not built |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Archived `servers/gitlab` | Official GitLab MCP | zereight/gitlab-mcp | Our Approach |
|---------|--------------------------|---------------------|---------------------|--------------|
| Project discovery | `search_repositories` | `list_projects`, `get_project`, `list_groups` | many | `list_projects` + `get_project`; no groups |
| File read | `get_file_contents` (file or dir) | `get_repository_file` (offset/limit) + `list_repository_tree` | `get_file_contents`, `get_repository_tree` | Separate tree and file tools (clearer schema than the overloaded file-or-dir tool) |
| Branches | `create_branch` | `list_branches`, `add_branch` | yes | list + create |
| Commits | none (only via push) | `list_commits`, `get_commit`, `add_commit` | `list_commits`, `get_commit_diff`, `push_files` | list, get (with diff), multi-file commit |
| Code search | none | `search` (multi-scope), `semantic_search` | `search_code`, `search_project_code` | `search_code` blobs only |
| MR list/detail | none / none | `list_merge_requests`, `get_merge_request` (`include`) | yes | plain list + detail |
| MR diff | none | `get_merge_request_diffs` (paginated) | `list_merge_request_changed_files` + `get_merge_request_file_diff` | paginated diffs with visible truncation flags |
| MR comments | none | `save_note`, `get_merge_request_notes`, `save_merge_request_review` | notes, threads, drafts | list notes + create note; inline later |
| MR create/update | `create_merge_request` | `save_merge_request` (create or update) | yes | `create_merge_request` first; `update` in v1.x |
| Merge | none | `accept_merge_request` | `merge_merge_request` | `merge_merge_request` with readable failures |
| File write | `create_or_update_file`, `push_files` | `add_commit` | both | both, one shared primitive |
| Read-only/permission mode | none | via OAuth scopes | `GITLAB_PERMISSION_MODE` | Deliberately none (author decision) |
| Tool count | 9 | ~50 (beta, growing) | 261 | ~18 at v1 |

## Sources

- GitLab official MCP server tool reference (fetched 2026-09-24): https://docs.gitlab.com/user/model_context_protocol/mcp_server_tools/ — HIGH confidence for tool names/parameters
- GitLab official MCP server overview (Beta status, OAuth DCR, HTTP and `mcp-remote` stdio, Free/Premium/Ultimate): https://docs.gitlab.com/user/model_context_protocol/mcp_server/ — HIGH
- Archived reference server (archived 2025-05-29, 9 tools): https://github.com/modelcontextprotocol/servers-archived/tree/main/src/gitlab — HIGH
- zereight/gitlab-mcp README (261 tools, permission modes, 2-step MR review, transports): https://github.com/zereight/gitlab-mcp — MEDIUM (README-summary via fetch; the ~261 star count is LOW and irrelevant to decisions)
- Other community listing: kopfrechner/gitlab-mr-mcp (https://github.com/kopfrechner/gitlab-mr-mcp), Glama/mcp.directory listings — LOW-MEDIUM
- GitLab REST API docs, Merge Requests (`detailed_merge_status`, `changes_count` cap 1000+, `diff_refs`, merge endpoint): https://docs.gitlab.com/api/merge_requests/ — HIGH for the cited points
- GitLab REST API docs, Commits (`actions[]` with create/delete/move/update/chmod, `start_branch`, `last_commit_id`): https://docs.gitlab.com/api/commits/ — HIGH

### Not verified (flag for phase-level research)

- Whether `GET /merge_requests/:iid/diffs` is the current recommended paginated endpoint and `/changes` is deprecated (recall from training, not confirmed by the page fetched).
- Behavior of global/group `scope=blobs` search on gitlab.com for a free-tier PAT (basic vs advanced/zoekt search availability).
- Exact single-file API verbs and `last_commit_id`/`execute_filemode` requirements on `repository/files` (create=POST, update=PUT) — confirm in the phase that implements writes.
- Whether the agent's MCP SDK 1.30.x client forwards tool annotations and handles structured output (relevant only to P2/P3 items).
- Whether an official PAT path exists for the official server (the fetched page documents OAuth DCR only); not needed for our design.

---
*Feature research for: GitLab MCP server (code reading, Merge Requests, repository writes)*
*Researched: 2026-09-24*
