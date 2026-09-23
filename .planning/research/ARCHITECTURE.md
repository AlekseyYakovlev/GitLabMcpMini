# Architecture Research

**Domain:** stdio MCP server wrapping a REST API (GitLab REST API v4, gitlab.com, PAT auth)
**Researched:** 2026-09-24
**Confidence:** MEDIUM-HIGH (MCP spec, SDK docs, GitLab rate-limit docs verified; layout/testing conventions are ecosystem consensus plus my own judgement, marked where relevant)

## Scope Note

Language (Go vs Python) is decided by STACK.md, not here. The component boundaries, data flow and build order below are language-neutral. The concrete folder layout is given for Go (official `modelcontextprotocol/go-sdk`, v1.8.0 as of 2026-09-04) with a Python mapping table, because Go is the natural fit for the "single .exe, no runtime" constraint.

Version watch-out: the official Python SDK is now on a **v2 line** (`MCPServer`, `Client`, spec 2026-07-28), while the client agent (AiAdventAgentV2) pins `mcp` 1.30.x. If Python is chosen, pin the server to the same major as the agent (`mcp>=1.30,<2`) unless the STACK researcher verifies v2 servers negotiate cleanly with a 1.30 client. The Go SDK supports protocol versions 2024-11-05 through 2026-07-28 with negotiation, so a 1.30 client is fine (HIGH for Go SDK claim, per its README; the actual 1.30 client handshake must be verified in the e2e test).

## Standard Architecture

### System Overview

```
 MCP client (agent process)                     gitlab.com
   │  JSON-RPC, newline-delimited                   ▲
   │  stdin ▼        ▲ stdout (protocol ONLY)       │ HTTPS, PRIVATE-TOKEN header
┌──┴─────────────────┴───────────────────────────────┴────────────────┐
│ PROCESS: gitlab-mcp                                                  │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ L1  Transport + Protocol   (SDK-owned, we only wire it)        │  │
│  │     stdio transport, initialize, tools/list, tools/call,       │  │
│  │     input-schema validation, JSON-RPC framing                  │  │
│  └───────────────────────────┬────────────────────────────────────┘  │
│                              │ typed args in / (result, error) out   │
│  ┌───────────────────────────▼────────────────────────────────────┐  │
│  │ L2  Tool layer (internal/tools)                                │  │
│  │     registry + one handler per tool + input structs (=schema)  │  │
│  │     formatting/truncation of output, error -> isError mapping  │  │
│  └───────────────────────────┬────────────────────────────────────┘  │
│                              │ plain Go calls, ctx propagated        │
│  ┌───────────────────────────▼────────────────────────────────────┐  │
│  │ L3  GitLab API client (internal/gitlab)                        │  │
│  │     auth, URL/path-encoding, JSON decode, pagination,          │  │
│  │     retry/rate-limit, typed *APIError, no MCP knowledge        │  │
│  └───────────────────────────┬────────────────────────────────────┘  │
│                              │ net/http                              │
│  ┌───────────────────────────▼────────────────────────────────────┐  │
│  │ L4  HTTP (stdlib http.Client, timeouts, User-Agent)            │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Cross-cutting: config (env -> struct, once at startup)              │
│                 logging (slog -> os.Stderr ONLY)                     │
└──────────────────────────────────────────────────────────────────────┘
```

Dependency direction is strictly downward: `tools -> gitlab -> net/http`. The `gitlab` package never imports the MCP SDK; the `tools` package never builds raw HTTP requests. This is what makes each layer testable on its own.

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| `cmd/gitlab-mcp/main.go` (composition root) | Load config, build logger, build GitLab client, build server, register tools, run stdio transport, handle SIGINT/stdin-EOF shutdown | ~50 lines; the only place that knows all components; no logic |
| `config` | Read `GITLAB_TOKEN` (required), `GITLAB_URL` (default `https://gitlab.com`, needed as test seam), timeouts, `LOG_LEVEL`; validate; never log the token | Struct + `Load(getenv)`; take `getenv` func so tests do not touch real env |
| `logging` | Structured logs to **stderr only**; redact token | `slog.New(slog.NewTextHandler(os.Stderr, ...))`; set as default; ban `fmt.Print*`/`log` to stdout via lint (`forbidigo`) |
| MCP server wiring (`internal/server`) | Create `mcp.Server` (name/version), register all tools, expose `New(client, logger) *mcp.Server` so tests can attach any transport | Thin; `Run(ctx, &mcp.StdioTransport{})` is called from main only |
| Tool registry + handlers (`internal/tools`) | One file per domain group (projects/read, MRs, write); each tool = name, description, annotations, input struct, handler | `mcp.AddTool(server, &mcp.Tool{...}, handler)`; input JSON schema is inferred from the struct (`jsonschema:"..."` tags) |
| Result formatting (`tools/format.go`) | Turn API structs into compact, LLM-friendly text/JSON; truncate big blobs and say so; attach `next_page` hints | Pure functions, table-tested |
| Error mapping (`tools/errors.go`) | `*gitlab.APIError` / network error / ctx error -> actionable one-line message returned as tool error (`isError: true`) | Single `toToolError(err)` used by every handler |
| GitLab client (`internal/gitlab`) | Base URL, `PRIVATE-TOKEN` header, `do(ctx, method, path, query, body, out)`, project-ID/path encoding, pagination helpers, retry on 429/5xx (GET only), typed errors, rate-limit header capture | Own ~300 lines on `net/http`; endpoint methods grouped per file (projects.go, repository.go, commits.go, mergerequests.go) |
| Test harness (`internal/testutil`, `test/e2e`) | Fake GitLab (`httptest.Server`), in-memory MCP session helper, subprocess stdio client helper | See Testing Architecture |

## Recommended Project Structure

```
gitlab-mcp/
├── cmd/
│   └── gitlab-mcp/
│       └── main.go            # composition root only
├── internal/
│   ├── config/                # env -> Config, validation
│   │   └── config_test.go
│   ├── logging/               # stderr slog setup, token redaction
│   ├── gitlab/                # REST client; NO import of MCP SDK
│   │   ├── client.go          # New(baseURL, token, opts), do(), retry, rate-limit
│   │   ├── errors.go          # APIError{Status, Message, RetryAfter}, sentinel helpers
│   │   ├── pagination.go      # Page{Items, NextPage}, per_page clamp (max 100)
│   │   ├── projects.go        # ListProjects, GetProject
│   │   ├── repository.go      # Tree, GetFile, Branches, CreateBranch, Search
│   │   ├── commits.go         # ListCommits, CreateCommit(actions[])
│   │   ├── mergerequests.go   # List/Get/Diffs/Create/Note/Merge
│   │   └── *_test.go          # httptest-based, one fake per endpoint family
│   ├── tools/                 # MCP tool layer
│   │   ├── register.go        # Register(server, client) -> calls each group
│   │   ├── read.go            # projects, tree, file, branches, commits, search
│   │   ├── mergerequests.go
│   │   ├── write.go           # create/update file, commit, branch
│   │   ├── errors.go          # toToolError
│   │   ├── format.go          # truncation, text rendering
│   │   └── *_test.go          # in-memory MCP session + fake GitLab
│   ├── server/
│   │   └── server.go          # New(...) *mcp.Server
│   └── testutil/
│       ├── fakegitlab.go      # httptest.Server with route table + request recorder
│       └── session.go         # in-memory client<->server session helper
├── test/
│   └── e2e/
│       ├── stdio_test.go      # builds binary, spawns it, real stdio client vs fake GitLab
│       └── live_test.go       # //go:build live — real gitlab.com smoke, opt-in
├── go.mod
└── README.md                  # how to register with the agent (command, env)
```

### Structure Rationale

- **`internal/gitlab` separate from `internal/tools`:** the REST client is a reusable, MCP-free library; error semantics (status, Retry-After) live there, presentation semantics (LLM-facing wording) live in tools. Prevents the classic mistake of building HTTP calls inside handlers.
- **`internal/server.New` returns the server without running it:** lets tests bind in-memory or subprocess transports to the identical object graph. Only `main` calls `Run` with stdio.
- **`GITLAB_URL` config even though only gitlab.com is supported:** it is the seam that lets the *real binary* be tested over real stdio against a local fake GitLab. Not a self-hosted feature; keep it undocumented-as-supported.
- **One tools file per domain group, not per tool:** ~20 tools total; per-tool files are noise for a "Mini" project.
- **Python mapping (if STACK picks Python):** `internal/gitlab` -> `gitlab_mcp/client.py` (httpx.AsyncClient, `respx` or `httpx.MockTransport` for tests); `internal/tools` -> `gitlab_mcp/tools/*.py` with `@mcp.tool()` decorators (schema from type hints/docstrings); `server.New` -> `build_server(client)`; e2e via `mcp.client.stdio.stdio_client` + `StdioServerParameters`; in-memory via SDK's in-memory `Client(server)` (v2) or `create_connected_server_and_client_session` (v1.x).

## Architectural Patterns

### Pattern 1: Thin tool handlers over a typed client

**What:** Each handler does exactly: (1) apply defaults/clamp inputs, (2) one or two client calls, (3) format, (4) map errors. No HTTP, no JSON wrangling, no business rules beyond input shaping.
**When to use:** Always; this is the standard shape of every well-structured API-wrapping MCP server.
**Trade-offs:** Slight boilerplate per tool; gains uniform error/format behavior and trivially small handlers.

**Example (Go, illustrative):**
```go
type GetFileInput struct {
    Project string `json:"project" jsonschema:"project ID or full path, e.g. group/repo"`
    Path    string `json:"path"    jsonschema:"file path within the repository"`
    Ref     string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA; default branch if empty"`
}

func getFile(c *gitlab.Client) mcp.ToolHandlerFor[GetFileInput, any] {
    return func(ctx context.Context, _ *mcp.CallToolRequest, in GetFileInput) (*mcp.CallToolResult, any, error) {
        f, err := c.GetFile(ctx, in.Project, in.Path, orHEAD(in.Ref))
        if err != nil {
            return toolError(err), nil, nil   // isError:true with actionable text
        }
        return textResult(format.File(f, maxBytes)), nil, nil
    }
}
```

### Pattern 2: Two error channels, chosen by "could the model fix this?"

**What:** MCP defines protocol errors (JSON-RPC error: unknown tool, malformed request) and tool execution errors (`isError: true` in the result). Everything that originates from GitLab or the network is a **tool execution error** so the model sees it and can self-correct. Protocol errors are left to the SDK (schema validation failures, unknown tool). The process must never crash or write a stack trace to stdout on any API failure; also recover panics per handler (the Go SDK returns handler `error`s as `isError` results; Python SDK turns unhandled exceptions into a generic `isError` and logs the traceback).
**When to use:** Every handler, via one shared mapper.
**Trade-offs:** None worth avoiding; this is what the spec says (HIGH, MCP spec 2025-11-25 tools/Error Handling).

**Error mapping table (put in `tools/errors.go`):**

| Source | Tool-facing message (one line, actionable) | Retry in client? |
|--------|--------------------------------------------|------------------|
| 401 | "GitLab rejected the token (401). Check GITLAB_TOKEN is valid/not expired." | No |
| 403 | "Forbidden (403): token lacks permission/scope (needs `api` for writes, `read_api`/`read_repository` for reads) or the role is insufficient." | No |
| 404 | "Not found (404): project/path/ref/MR does not exist, or the token cannot see it (GitLab returns 404 for inaccessible private resources)." | No |
| 400 / 409 / 422 | Pass GitLab's `message`/`error` verbatim (branch exists, file exists, cannot merge, conflicts) | No |
| 405 / 406 on MR merge | "MR cannot be merged (not mergeable / checks pending / conflicts)" plus GitLab message | No |
| 429 | Client sleeps `Retry-After` (bounded, e.g. <= 10 s, GET only) then retries once or twice; if still 429: "Rate limited by GitLab; retry after N s." | Yes, bounded |
| 5xx | Retry idempotent GETs with short backoff (2-3 attempts); otherwise "GitLab server error (5xx)" | GET only |
| Network/DNS/TLS/timeout | "Could not reach gitlab.com: <cause>" ; distinguish ctx cancellation (client cancelled) from timeout | GET only |
| Malformed JSON from GitLab | "Unexpected response from GitLab" + log body snippet to stderr | No |

**Never auto-retry POST/PUT/DELETE** (commit, create branch, merge, comment): a timeout after the server applied the change would duplicate it. Surface the error and let the model/user check state.

### Pattern 3: Pagination surfaced, not swallowed

**What:** GitLab uses offset pagination by default (`page`, `per_page` max 100, default 20; headers `X-Next-Page`, `X-Total-Pages`, `Link`) and keyset for large collections; `X-Total`/`X-Total-Pages` are omitted above 10,000 records (HIGH, GitLab REST docs). List tools accept `page` and `per_page` (clamped, default ~20-30), return the items plus an explicit `next_page` value (from `X-Next-Page`) in the output. Do **not** auto-walk all pages inside one tool call; that burns rate limit, latency and LLM context.
**When to use:** All list-type tools (projects, tree, branches, commits, MRs, search results, diffs).
**Trade-offs:** Model needs extra calls for page 2+, but output stays bounded and the model controls the cost. Optional internal helper `AllPages(max N)` only for cases that truly need it (e.g. resolving a branch by name).

### Pattern 4: Bounded, LLM-shaped output

**What:** Responses go into the model's context. Return only useful fields (not the 60-field GitLab object), render trees/commits/MRs as compact text or slim JSON, and hard-cap large payloads: file content (e.g. 100-200 KB or N lines with a "truncated, total X bytes; use start_line/end_line" note), MR diffs (per-file cap; GitLab flags `too_large`/`collapsed` diffs and the single-commit diff endpoint stops paginating at its max-files limit), search results (limit count). Binary files: report "binary, N bytes" instead of dumping base64.
**When to use:** Every read tool. Highest-value design decision for an agent-facing server.
**Trade-offs:** Slightly more formatting code; avoids context blowups and huge stdout messages.

### Pattern 5: Project reference normalization in the client

**What:** Accept `project` as numeric ID **or** `group/sub/repo` path. The client URL-encodes the path as a single segment (`group%2Frepo`). File paths in the Repository Files API must also be URL-encoded (`lib%2Fclass%2Erb`) (HIGH, GitLab docs). In Go, setting `URL.Path` alone will re-decode `%2F`; construct the request URL with `RawPath` (or build the string and `url.Parse` it) and cover this with a test that asserts the on-the-wire path.
**When to use:** Centralize in one helper; every endpoint method calls it.
**Trade-offs:** None; forgetting it yields sporadic 404s on nested groups and files in subfolders.

### Pattern 6: Tool annotations for read vs write

**What:** Set `readOnlyHint: true` on all read tools and `destructiveHint`/`readOnlyHint: false` on writes/merge. The project owner chose no read-only mode, but hints are free metadata and help the agent's UI/policy layer; the spec says clients must treat annotations as untrusted, so they are advisory only (HIGH).
**Naming:** prefix all tools `gitlab_` (`gitlab_get_file`, `gitlab_list_merge_requests`, ...). The agent already runs other servers (e.g. Go `filesystem.exe` exposing `read_file`, `list_directory`-style names); prefixing avoids collisions in a flat tool namespace. Allowed characters per spec: `[A-Za-z0-9_.-]`, 1-128 chars.

## Data Flow

### Request Flow (tools/call)

```
Agent (MCP client)
  │ tools/call {name, arguments}            [stdin, one JSON line]
  ▼
SDK transport ──► JSON-RPC dispatch ──► validate arguments vs inputSchema
  │                                         │ invalid -> JSON-RPC error (-32602) / isError
  ▼
tools.handler(ctx, args)
  │ clamp/default inputs
  ▼
gitlab.Client.Method(ctx, ...)
  │ build URL (encoded IDs/paths) + PRIVATE-TOKEN header
  ▼
http.Client.Do ──► gitlab.com
  │◄── status, body, headers (RateLimit-*, X-Next-Page, Retry-After)
  ▼
client: 2xx -> decode JSON (+ base64 for file content) -> typed struct + NextPage
        429/5xx(GET) -> sleep/retry (bounded, ctx-aware)
        other non-2xx -> *APIError{Status, Message}
  ▼
tools: format/truncate -> CallToolResult{content:[text], isError:false}
       or toToolError(err) -> CallToolResult{content:[text], isError:true}
  ▼
SDK ──► JSON-RPC response ──► [stdout, one JSON line] ──► Agent
```

Logs (request method/path/status/duration, retries, never the token or bodies by default) go to **stderr** at every step.

### Lifecycle Flow

```
Agent spawns process (cmd, env: GITLAB_TOKEN)
  main: config.Load -> fail fast to stderr + exit(1) if token missing
        logging -> stderr; client.New; server.New; tools.Register
        server.Run(ctx, StdioTransport)      // blocks
  client: initialize -> server: capabilities {tools} + serverInfo
  client: notifications/initialized
  client: tools/list -> registry (no network, no token check needed)
  client: tools/call ... (repeat)
  shutdown: client closes stdin (EOF) -> Run returns -> flush logs -> exit 0
```

Design choices worth stating explicitly:
- **Handshake and `tools/list` must not depend on network.** Do not call GitLab at startup. An optional `gitlab_whoami` tool (`GET /user`) gives a cheap token check on demand.
- **Missing token:** fail fast on stderr with a clear message (agents typically capture stderr). Rationale: an "everything returns 401" server is harder to diagnose than a process that exits with a message. (My judgement; alternative is to start and return a tool error per call. Either is defensible; pick one and test it.)
- **Concurrency:** the SDK may dispatch requests concurrently; the client must be goroutine-safe (`http.Client` is) and hold no mutable per-request state. Rate-limit state (if tracked) needs a mutex.
- **Context:** propagate the handler `ctx` into every HTTP request so MCP cancellation and shutdown abort in-flight calls; add a per-request timeout (e.g. 30 s) independent of ctx.

### State Management

No persistent state. Only process-lifetime state: the config struct, the shared `http.Client` (connection reuse), optionally last-seen `RateLimit-Remaining` for proactive slowdown. No caching in v1 (staleness bugs for a personal tool are worse than extra calls). If a cache is ever added, TTL and scope must be explicit, and never for write-adjacent reads.

### Key Data Flows

1. **Read path (tree/file/branches/commits/search):** args -> client GET -> decode -> compact text with `next_page`. File content: GitLab returns base64 in the JSON file object (also `blob_id`, `last_commit_id`, `size`); the client decodes it, the tool truncates and detects binary. The `/raw` endpoint is an alternative for plain content but loses the metadata needed later for optimistic-concurrency updates (`last_commit_id`), so prefer the JSON endpoint (MEDIUM: from GitLab Repository Files docs summary).
2. **MR path:** list (project-scoped, filters `state`, `author`), get (note `detailed_merge_status` is computed asynchronously and may lag right after creation/push; do not treat a transient value as final), diffs (paginated per-file; cap and flag `too_large`), create (needs source branch already pushed), comment (notes endpoint), merge (`PUT .../merge`; pre-check status only for messaging, rely on GitLab's 405/406 for truth).
3. **Write path:** prefer the **Commits API with `actions[]`** (`create|update|delete|move|chmod`, plus `start_branch` to create a branch in the same call) as the single primitive for multi-file changes; single-file create/update can wrap the Repository Files API (`POST`/`PUT`, needs `branch`, `commit_message`, `content`, optional `start_branch`, `last_commit_id`). Branch creation is its own endpoint. Large requests are size/rate limited (over 20 MB: 3 req/30 s) so cap payload size in the tool. Writes are never retried.

## Testing Architecture

Four tiers, each catching a different class of bug. All but the last are hermetic (no network, no token).

| Tier | What is under test | How | Catches |
|------|--------------------|-----|---------|
| 1. Client unit | `internal/gitlab` | `httptest.Server` returning canned GitLab JSON/headers; assert request method, **wire path (encoded)**, headers, query; cases: 200, pagination headers, 401/403/404/409/422, 429 with `Retry-After`, 5xx then success, timeout, ctx cancel, malformed JSON | Encoding bugs, pagination, retry/error mapping |
| 2. Tool unit (in-memory MCP) | `internal/tools` + `server.New` | Fake GitLab (`testutil.fakegitlab`) + SDK in-memory transport: Go `mcp.NewInMemoryTransports()` + `mcp.NewClient(...).Connect(ctx, t, nil)`; Python `Client(server)` (v2) / `create_connected_server_and_client_session` (v1). Call `tools/list` and `tools/call`, assert schemas, content, `isError` | Schema shape, handler wiring, error-to-`isError` mapping, output truncation |
| 3. Stdio e2e (real subprocess) | The **built binary** over real stdin/stdout | `TestMain` runs `go build -o $TMP/gitlab-mcp.exe`; test starts a fake GitLab `httptest.Server`, spawns the binary with env `GITLAB_TOKEN=test`, `GITLAB_URL=<fake>`, and connects with the SDK client over `&mcp.CommandTransport{Command: exec.Command(bin)}`; assert initialize -> tools/list -> a read call -> a write call -> error call. Python equivalent: `stdio_client(StdioServerParameters(command=..., env=...))` + `ClientSession` | The failure class in-memory tests cannot see: stray stdout writes, buffering, Windows CRLF/encoding, exit-on-EOF, env handling, startup crash |
| 3b. Raw-protocol guard | stdout purity | Same subprocess; a second test reads stdout line-by-line with a plain JSON parser and asserts every line is valid JSON-RPC and nothing else appears (also run a tool that triggers a logged warning/error path) | Any `print`/`fmt.Println`/library banner on stdout |
| 4. Live smoke (opt-in) | Real gitlab.com | Build tag `live` (or env `GITLAB_LIVE=1`) + real `GITLAB_TOKEN`; runs the same stdio client against a **dedicated scratch project** owned by the author; sequence: read tree/file -> create branch -> commit file -> create MR -> comment -> merge -> cleanup. Never runs in default `go test ./...` | Real API drift, permission/scope issues, rate-limit behavior, the Core Value ("works on real gitlab.com") |

Manual checks, in addition to automated tiers (satisfies the "ручная проверка через stdio-клиент" requirement):
- **MCP Inspector** (`npx @modelcontextprotocol/inspector`, UI or `--cli`) pointed at the binary: a dev-time tool only; Node is needed just on the developer machine, not for the shipped server or the agent runtime. (MEDIUM: standard practice; verify exact CLI flags at implementation.)
- A tiny checked-in **stdio smoke script/CLI** (Go or Python using the SDK client) that spawns the binary, lists tools, and calls one tool with args from the command line: the fastest repeatable way to exercise real gitlab.com and the reference for how the agent will see the server.
- Finally, exercising against the agent's own MCP client version (`mcp` 1.30.x) is out of project scope, but a 20-line Python `stdio_client` script using the same pinned `mcp` 1.30.x validates protocol/inputSchema compatibility cheaply and is worth including as a compatibility test (recommended, because the compatibility constraint is explicit in PROJECT.md).

Test seams summary: `GITLAB_URL` (base URL) makes tier 3 hermetic; `config.Load(getenv)` avoids env pollution; `server.New(client, logger)` makes tier 2 transport-agnostic; the fake GitLab records requests so tests assert *what was sent to GitLab* (e.g. that a commit sent the right `actions[]`), not only what came back.

## Suggested Build Order (dependencies)

```
config ─┐
logging ┼─► server.New + 1 trivial tool ─► [E2E stdio harness]   (Phase A: walking skeleton)
        │
gitlab client core (do, errors, pagination, retry) ─► client unit tests   (Phase B)
        │
        ├─► read tools (projects, tree, file, branches, commits, search)   (Phase C)
        │        │
        │        ├─► MR tools (list/get/diffs; then create/comment/merge)  (Phase D)
        │        └─► write tools (commit actions, file create/update, branch) (Phase E)
        │
        └─► hardening: truncation limits, live smoke, README/agent config   (Phase F)
```

1. **Phase A: Walking skeleton.** config + stderr logging + `server.New` + a single no-network tool (e.g. `gitlab_ping`/server info) + the **subprocess stdio e2e harness and stdout-purity test**. Rationale: the Core Value is "correct MCP handshake over stdio"; prove it first against the agent-pinned client version, and get the fake-GitLab/e2e scaffolding in place before any real tool exists. Highest-risk-first for protocol, cheapest to fix now.
2. **Phase B: GitLab client core.** auth header, `do()`, project/path encoding, error types, pagination parsing, bounded retry (GET), rate-limit headers, with full httptest coverage. Everything downstream depends on it; nothing downstream should re-implement any of it.
3. **Phase C: Read tools.** projects list/search, tree, get file, branches, commits, code search. Establishes the output-shaping conventions (truncation, `next_page`, error mapper) that later tools copy. Read tools are also the live-verification fixture for everything after.
4. **Phase D: Merge Request tools.** Read side first (list, get, diffs), then mutating side (create, comment, merge). MR create needs a pushed source branch with changes, so its live test depends on Phase E, or on a pre-seeded branch in the scratch project.
5. **Phase E: Write tools.** Commit-with-actions primitive first, then file create/update and branch create as thin wrappers. Built after read tools because verification of writes uses read tools; built with no retries and with pre-write validation only where cheap.
6. **Phase F: Hardening and delivery.** Output caps tuned against real data, live smoke suite, error-message review against real 401/403/404/429 cases, README with the exact agent `command`/`env` snippet, Windows build (`GOOS=windows`, `.exe`).

Phases D and E can be swapped or run in parallel once C is done; E before D's create/merge is slightly better for live testing. The critical path is A -> B -> C.

## Scaling Considerations

One user, one process, sequential agent calls. "Scale" here is per-call cost, not users.

| Concern | Actual limit | Approach |
|---------|--------------|----------|
| GitLab.com rate limit | Authenticated API: per-plan hourly (Free 5,000/h) plus per-minute burst (Free 100/min); Files API 500 req/min per IP+file path; search 100 req/min per user; large blobs (>10 MB) 5 req/min (HIGH, GitLab rate-limit docs) | Honor `Retry-After` on 429; read `RateLimit-Remaining` and log at low values; avoid multi-page auto-walks and N+1 fan-out (e.g. per-MR detail calls inside list tools) |
| LLM context budget | The real bottleneck | Slim fields, per-tool caps, pagination hints (Pattern 4) |
| Latency | Each call is a network round-trip to gitlab.com | One shared `http.Client` with keep-alive; parallelize only independent sub-requests within a tool, bounded |
| Large repos/files | 10,000+ records lose totals; keyset pagination recommended for big lists | Offset paging is fine for a personal tool; keep `per_page` <= 100; switch a specific endpoint to keyset only if it proves slow |

### Scaling Priorities

1. **First bottleneck:** context size of tool outputs (diffs, files, trees). Fix with caps and pagination, not caching.
2. **Second bottleneck:** the 100/min burst limit on Free plan under chatty agent loops. Fix with `Retry-After` handling and reducing fan-out.

## Anti-Patterns

### Anti-Pattern 1: Anything on stdout except protocol

**What people do:** `fmt.Println`/`print()` debugging, library banners, default loggers, uncaught panic traces, Rich-style console loggers that default to stdout.
**Why it's wrong:** stdout is the JSON-RPC channel; the spec says the server MUST NOT write anything else there (HIGH, MCP transports spec). One stray line breaks the client's parse, often with an opaque "connection closed" error. Python SDKs partially divert flushed stdout, but do not rely on that.
**Do this instead:** stderr-only logging configured first thing in `main`; lint-ban stdout printing; tier-3b stdout-purity test; in Go consider redirecting `os.Stdout` to `os.Stderr` after the transport has captured the real stdout, if the SDK allows an explicit reader/writer transport (verify in SDK docs; LOW confidence on the exact API).

### Anti-Pattern 2: Crashing or returning protocol errors for API failures

**What people do:** Let HTTP errors bubble up as exceptions/panics or JSON-RPC errors.
**Why it's wrong:** Kills the subprocess (agent must respawn) or hides the message from the model, which cannot self-correct.
**Do this instead:** Pattern 2: `isError: true` with an actionable sentence; recover from panics per handler; log details on stderr.

### Anti-Pattern 3: One generic "call any GitLab endpoint" tool

**What people do:** Expose `gitlab_request(method, path, body)` to save effort.
**Why it's wrong:** Defeats the purpose of typed tools (schema-guided arguments, curated output, annotations), makes writes indistinguishable from reads, and floods context with raw JSON. Also not the "compact set of core tools" the project defines.
**Do this instead:** ~15-25 purpose-specific tools with tight input schemas.

### Anti-Pattern 4: Dumping raw GitLab JSON and unbounded content

**What people do:** Return the API response verbatim; return whole files/diffs.
**Why it's wrong:** Burns context, may include noise/PII fields, slows the agent, can exceed client message limits.
**Do this instead:** Pattern 4.

### Anti-Pattern 5: Auto-retrying non-idempotent calls, or retrying without bounds

**What people do:** Blanket retry middleware on all methods, infinite backoff on 429.
**Why it's wrong:** Duplicate commits/comments/merges; the agent appears hung while the server sleeps.
**Do this instead:** Retry GET only, cap total wait (e.g. <= 10-15 s), make sleeping ctx-aware, then return an error stating the retry time.

### Anti-Pattern 6: Mocking the client interface instead of the HTTP boundary

**What people do:** Hand-written mock of a `GitLabAPI` interface in tool tests.
**Why it's wrong:** Tests pass while the real client sends a wrong path/encoding/param; more mocks to maintain in a project this small.
**Do this instead:** Tools depend on the concrete client; tests point the real client at an `httptest` fake GitLab and assert on recorded requests. Introduce an interface only if a second implementation appears.

### Anti-Pattern 7: Token leakage

**What people do:** Log request headers/URLs with the token, echo config at startup, include it in error text.
**Why it's wrong:** Logs go to the agent's captured stderr and possibly to files. PAT grants (by author's decision) unrestricted write access.
**Do this instead:** Redact in the logging layer; never print config; error messages contain GitLab's message only. Note tokens should be minimal-scope (`api` needed for writes) since the project deliberately has no read-only mode.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| gitlab.com REST v4 | `https://gitlab.com/api/v4`, header `PRIVATE-TOKEN: <PAT>` (HIGH) | 429 body is plain text ("Retry later"), not JSON; error bodies elsewhere are JSON `{"message": ...}` or `{"error": ...}`; handle both plus non-JSON. `Retry-After`, `RateLimit-Limit/Remaining/Reset` headers present |
| Agent (AiAdventAgentV2, MCP client `mcp` 1.30.x) | Spawns the binary/script as a stdio subprocess with env (`GITLAB_TOKEN`) | Out of project scope to wire, but the README should document command + env; agent captures stderr; tool names should not collide with `filesystem.exe` tools |
| MCP SDK | Official Go SDK v1.8.0 (or Python SDK) | SDK does JSON-RPC, lifecycle, schema inference/validation; we own tools and client. Protocol versions negotiated (Go SDK supports 2024-11-05..2026-07-28) |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `main` -> all | Constructor injection | Only place components are assembled; no globals besides the default logger |
| `tools` -> `gitlab` | Direct method calls with `ctx`, typed results, `*APIError` | `gitlab` knows nothing of MCP; `tools` knows nothing of HTTP |
| `server` -> SDK | `mcp.NewServer` + `mcp.AddTool` | The only files importing the SDK: `server`, `tools`, tests |
| `config`/`logging` -> everything | Values passed in (Config struct, `*slog.Logger`) | No package-level reads of `os.Getenv` outside `config` |

## Open Items / Confidence Notes

| Claim | Confidence | Note |
|-------|------------|------|
| stdio rules (newline-delimited, stdout protocol-only, stderr logging) | HIGH | MCP spec 2025-11-25 |
| Tool error model (`isError` vs protocol error) | HIGH | MCP spec + Go and Python SDK docs |
| Go SDK API names (`NewServer`, `AddTool`, `StdioTransport`, `CommandTransport{Command}`, `NewInMemoryTransports`, `ClientSession.CallTool`) | HIGH | pkg.go.dev v1.8.0 |
| Python SDK v2 in-memory `Client(server)`, `raise_exceptions` | HIGH | py.sdk docs; v1.x helper name from secondary sources, MEDIUM |
| GitLab pagination, rate limits, file path encoding, commit actions | HIGH | docs.gitlab.com |
| MR diffs endpoint name (`/merge_requests/:iid/diffs`, replacing deprecated `/changes`), merge status codes | MEDIUM | Not confirmed from fetched excerpt; confirm during MR phase |
| Go `os.Stdout` redirect trick / `IOTransport` availability | LOW | Verify in SDK docs before relying on it |
| Fail-fast on missing token vs lazy error | judgement | Either OK; decide in Phase A |
| MCP Inspector CLI flags | MEDIUM | Verify at implementation time |

## Sources

- MCP spec, Transports (stdio): https://modelcontextprotocol.io/specification/2025-11-25/basic/transports (HIGH)
- MCP spec, Tools (error handling, naming, annotations, schemas): https://modelcontextprotocol.io/specification/2025-11-25/server/tools (HIGH)
- Go SDK package docs (v1.8.0, 2026-09-04): https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp (HIGH)
- Go SDK repo/README (protocol versions, stdio example): https://github.com/modelcontextprotocol/go-sdk (HIGH)
- Go SDK server docs (tool errors, schema inference): https://go.sdk.modelcontextprotocol.io/server/ (HIGH)
- Python SDK docs (testing, error handling, v2 vs v1.x): https://py.sdk.modelcontextprotocol.io/get-started/testing/ , https://py.sdk.modelcontextprotocol.io/servers/handling-errors/ , https://github.com/modelcontextprotocol/python-sdk (HIGH)
- Python SDK logging/stdio guidance: https://py.sdk.modelcontextprotocol.io/handlers/logging/ and community reports of stdout corruption (LOW-MEDIUM)
- GitLab REST API (pagination, auth): https://docs.gitlab.com/api/rest/ (HIGH)
- GitLab.com rate limits: https://docs.gitlab.com/user/gitlab_com/rate_limits/ and https://docs.gitlab.com/administration/settings/user_and_ip_rate_limits/ (HIGH)
- GitLab Repository Files API: https://docs.gitlab.com/api/repository_files/ (HIGH)
- GitLab Commits API (actions[]): https://docs.gitlab.com/api/commits/ (HIGH)
- GitLab Merge Requests API: https://docs.gitlab.com/api/merge_requests/ (MEDIUM for diffs/merge details)

---
*Architecture research for: stdio MCP server wrapping the GitLab REST API*
*Researched: 2026-09-24*
