# Phase 1: Подключение и чтение проекта - Research

**Researched:** 2026-09-24
**Domain:** stdio MCP server (Go, `modelcontextprotocol/go-sdk`) wrapping GitLab REST v4 via `client-go/v2`, Windows dev box, client = Python `mcp` 1.30.x
**Confidence:** HIGH for the client-go / go-sdk behaviour (every load-bearing claim below was executed in a scratch spike on this machine); MEDIUM for live gitlab.com specifics that fake servers cannot prove (tagged inline)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

### Клиент GitLab
- **D-01:** Используется `gitlab.com/gitlab-org/api/client-go/v2` (v2.64.0, версия зафиксирована в `go.mod`), а не самописный `net/http`-клиент. Это решение автора вопреки рекомендации исследования (hand-rolled). Blocker «выбор клиента» из STATE.md закрыт этим решением, отдельный спайк не нужен.
- **D-02:** Ретраи только для GET (429 и 5xx): до 2 повторов, суммарное ожидание ≤10 с с учётом `Retry-After`, общий бюджет вызова ≤25 с. Дальше `isError` с сообщением вида «повторите через N с».
- **D-03:** Запросы записи (POST/PUT/DELETE) никогда не повторяются автоматически (FND-07). Встроенные ретраи client-go (go-retryablehttp) не должны применяться к записи: политику ретраев нужно настроить или заменить, а не полагаться на значения по умолчанию.
- **D-04:** Токен нигде не должен попадать в вывод и логи (FND-03): редактирование `glpat-…` централизованно в слое логирования и в текстах ошибок, включая ошибки, которые возвращает сам client-go.

### Имена инструментов
- **D-05:** Имена без префикса: `list_projects`, `get_project`, `list_repository_tree`, `get_file_contents`, `whoami`. Агент сам добавляет `mcp__<slug>__`, поэтому префикс `gitlab_` избыточен. Имена ≤30 символов, описания ≤900, схемы плоские (FND-08). Правки REQUIREMENTS.md не нужны.

### Формат вывода
- **D-06:** Инструменты возвращают компактный текст, а не JSON: одна строка на элемент, например `123 group/proj (main) — описание`. Одиночные объекты (`get_project`, `whoami`) тоже выводятся текстом с ключевыми полями, а не сырым объектом GitLab.
- **D-07:** Футер пагинации и обрезки — одна строка-подсказка для модели. Примеры: `[page 1, per_page 20 — есть следующая страница: вызовите с page=2]`, `[вывод обрезан на 15000 символах; сузьте путь или уменьшите per_page]`. Признак следующей страницы берётся из `X-Next-Page`; `X-Total` не используется (FND-06).
- **D-08:** Общий бюджет вывода ~15 000 символов (ниже отсечки агента 20 000). Важные данные идут первыми, а обрезка всегда явная, без молчаливой потери.

### Чтение файла
- **D-09:** `get_file_contents` без диапазона строк ограничен общим бюджетом ~15 000 символов. Большой файл возвращается головой с пометкой вида «обрезан, всего N строк; используйте start_line/end_line». Параметры `start_line`/`end_line` входят в схему инструмента.
- **D-10:** Бинарный файл определяется по NUL-байту или невалидному UTF-8 в начале. Возвращается только пометка без содержимого и без base64, например «файл бинарный, N байт, blob_id …».
- **D-11:** Если `ref` не указан, `get_file_contents` и `list_repository_tree` берут ветку по умолчанию проекта (`default_branch` из `GET /projects/:id`) и указывают её в выводе. Значение `HEAD` не используется.

### Уже зафиксировано в требованиях и STACK.md (не переоткрывать)
- **D-12:** Go 1.25+, `modelcontextprotocol/go-sdk` v1.8.0, один `gitlab-mcp.exe`, `CGO_ENABLED=0`. Только stdio; stdout содержит только JSON-RPC, логи только в stderr через `slog`. Без `fmt.Print*` в stdout.
- **D-13:** Без `GITLAB_TOKEN` сервер сразу завершается с понятным сообщением в stderr и кодом 1 (fail fast). `GITLAB_URL` необязателен (по умолчанию `https://gitlab.com`) и служит швом для hermetic-тестов.
- **D-14:** Проект указывается числовым ID или путём `group/subgroup/project`; путь кодируется через один нормализатор. Тесты проверяют путь на проводе (subgroup, пробел, `#`, `+`, `%`, кириллица, `feature/x`).
- **D-15:** Ошибки 401/403/404/429/5xx и сетевые сбои приходят как `isError: true` с короткими понятными сообщениями, процесс не падает. Опциональные аргументы — простые типы с `omitempty`, не указатели (иначе в схеме появляется `["null", ...]`).

### Claude's Discretion
- Раскладка пакетов и имя Go-модуля (ориентир: `cmd/gitlab-mcp`, `internal/{config,logging,tools,server,testutil}`; отдельный слой `internal/gitlab` нужен только если оборачивание client-go в него оправдано).
- Точные значения `per_page` по умолчанию (ориентир 20, максимум 100) и лимиты на рекурсивное дерево.
- Точный формат текстовых строк вывода и набор полей проекта/дерева.
- Способ ограничения ретраев client-go (`WithCustomRetry`/`WithHTTPClient` и т. п.) и таймаутов (ориентир: HTTP-таймаут и дедлайн вызова ≈25 с).
- Расположение и структура smoke-скрипта и остальной тестовой обвязки (ориентир: `scripts/` с venv или `uv run --with mcp==1.30.0`).

### Deferred Ideas (OUT OF SCOPE)
Идей вне объёма фазы не возникло.

Заметка для Phase 4 (README): client-go v2 тянет большой граф зависимостей; проверить размер итогового `.exe` (в спайке 9-12 МБ) и что сборка остаётся без cgo.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FND-01 | stdio MCP server, initialize + tools/list with client 1.30.x, no network at startup | Verified: go-sdk v1.8.0 negotiates 2025-11-25 with `mcp` 1.30.0; `gitlab.NewClient` does no I/O at construction (auth `Init` runs lazily inside `Do`); e2e test asserts fake GitLab saw 0 requests |
| FND-02 | stdout = JSON-RPC only, logs stderr, exit on stdin close | Verified: `mcp.IOTransport{Reader: os.Stdin, Writer: realStdout}` + `os.Stdout = os.Stderr` guard; stray `fmt.Println` landed on stderr; LF-only lines; exit code 0 within 2 ms of stdin close |
| FND-03 | Token from `GITLAB_TOKEN`, fail fast, never in output/logs | Fail-fast pattern verified (exit 1); client-go errors verified to NOT contain the token (header-only auth); centralized redactor still required as defence in depth |
| FND-04 | Project by numeric ID or `group/subgroup/project` | client-go encodes the project string itself (`PathEscape`, `.` -> `%2E`); normalizer must NOT pre-encode. Wire table below is verified |
| FND-05 | 401/403/404/405/409/422/429/5xx + network -> `isError`, no crash | Verified error shapes of client-go (404 is a message-less sentinel; 429/5xx bodies are echoed as "failed to parse unknown error format"); go-sdk does NOT recover handler panics (verified crash), so a per-handler `recover` wrapper is mandatory |
| FND-06 | `page`/`per_page`, next-page flag, ~15 000 char budget with explicit truncation | `Response.NextPage` (from `X-Next-Page`) + `Response.NextLink` fallback; rune-based budget helper |
| FND-07 | Call within ~25 s; writes never retried | Verified: default client-go client retries a failing POST 6 times over 11.8 s; replacement `CheckRetry`/`Backoff` verified to give 1 attempt for POST/PUT/DELETE and <=3 for GET |
| FND-08 | Names <=30, descriptions <=900, flat schemas | go-sdk inferred schema verified flat (`type/properties/required/additionalProperties:false`); guard test walks the schema for `$ref/$defs/anyOf/oneOf/"null"`; `jsonschema:"..."` tag must not start with `word=` (verified panic) |
| UTIL-01 | `whoami` | `Users.CurrentUser()` (`GET /user`); optional best-effort `PersonalAccessTokens.GetSinglePersonalAccessToken()` (`GET /personal_access_tokens/self`) adds scopes/expiry |
| READ-01 | `list_projects` (membership=true default) | `Projects.ListProjects` with `Membership`, `Simple`, `OrderBy`, `Search`, `ListOptions`; pointer option fields via `gitlab.Ptr` |
| READ-02 | `get_project` incl. default branch | `Projects.GetProject`; `Project.DefaultBranch`, `EmptyRepo` |
| READ-03 | `list_repository_tree` paginated | `Repositories.ListTree`; `TreeNode{ID,Name,Type,Path,Mode}` |
| READ-04 | `get_file_contents` (base64, line range, big/binary guards) | `RepositoryFiles.GetFile` returns base64 `Content`, `Size`, `BlobID`, `LastCommitID`; transport-level body cap verified |
| QA-01 | httptest fake GitLab + in-memory MCP + e2e on real binary incl. stdout purity | All three tiers exercised in the spike (see Test Harness Design) |
| QA-02 | Python `mcp` 1.30.0 smoke script mirroring the agent | Verified: `uv run` with a PEP 723 header `dependencies = ["mcp==1.30.0"]` works; system Python 3.13 also already has `mcp` 1.30.0 |
</phase_requirements>

## Summary

Phase 1 is a walking skeleton plus four read tools. Everything hard is at the boundaries: (1) protocol cleanliness on Windows stdio, (2) making `client-go/v2` behave like a small, predictable HTTP client (its defaults are hostile to a write-safe, 25-second-bounded, token-safe server), and (3) encoding paths and bounding output for an LLM client that cuts at 20 000 characters.

The author locked `client-go/v2` (D-01) against the earlier research recommendation, and the CONTEXT asked for three explicit checks. Results, all executed against a local `httptest` server with client-go v2.64.0: **(a) retries** - the default client retries a failing POST 5 times (6 attempts, 11.8 s) and retries PUT/DELETE too; `WithoutRetries()` is all-or-nothing; the right tool is `WithCustomRetry` + `WithCustomBackoff` + `WithCustomRetryMax(2)` with a GET/HEAD-only policy (verified: POST/PUT/DELETE = 1 attempt, GET 5xx = 3 attempts, GET 429 honours a short `Retry-After`, long `Retry-After` surfaces immediately). Also the built-in 429 backoff reads `RateLimit-Reset`, not `Retry-After`, and a built-in limiter can block calls, so both must be replaced. **(b) token in errors** - client-go authenticates only through the `PRIVATE-TOKEN` header; none of its error strings (HTTP errors, dial errors, deadline, invalid-header) contained the token in any test, so the risk is low, but a central redactor is still cheap and required by D-04. **(c) path encoding** - client-go builds `RawPath` itself and encodes every string argument once with `url.PathEscape` plus `.` -> `%2E`; the verified wire table is below. The only place to get it wrong is pre-encoding the project reference (double encoding) or passing a URL.

Two SDK facts change the plan versus the STACK.md spike: **go-sdk v1.8.0 does not recover handler panics** (the process died with a stack trace) and **a `jsonschema:"path=..."` tag panics at `AddTool`**. Both are cheap to guard but must be in the plan.

**Primary recommendation:** Build a thin `internal/glclient` package that owns client construction (retry policy, transport with timeouts + body cap, no-op limiter, redacting logger), project/path normalisation and error classification; wrap every tool handler in one generic `safe()` (recover + 25 s deadline + `isError` mapping + redaction + output budget); run the server on `mcp.IOTransport{os.Stdin, realStdout}` with `os.Stdout = os.Stderr`; and prove it with three test tiers plus a PEP 723 Python smoke script.

## Architectural Responsibility Map

Single local process, no browser/CDN tiers. "Tiers" here are the process layers.

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| JSON-RPC framing, initialize, tools/list, argument validation | go-sdk (L1) | - | SDK owns it; we only wire `AddTool`. Validation errors already come back as `isError` (verified) |
| Panic recovery, 25 s deadline, error->text, redaction, output budget | Tool layer (`safe()` wrapper) | - | SDK gives none of these; one wrapper keeps handlers thin |
| Project/path normalisation | `glclient` | - | Must be a single function; tools never build paths |
| Retry policy, timeouts, body cap, auth header | `glclient` (client-go options + `http.Transport`) | - | D-02/D-03; policy must be fail-safe by default (GET-only) |
| Error classification (401/403/404/429/5xx/network) | `glclient` (typed `*Error`) | Tool layer (wording per tool) | Status/Retry-After live where the response is; wording depends on the tool ("file not found" vs "project not found") |
| Output shaping (one line per item, footer, 15 000 rune budget) | Tool layer (`format.go`) | - | Presentation is MCP/LLM-specific |
| Default-branch resolution | Tool layer via `glclient` | - | D-11; one extra `GET /projects/:id` when `ref` is empty |
| Token storage/redaction | `config` + `logging` | `safe()` | Token read once in `main`; redactor applied to logs and to final tool text |
| Persistence | none | - | No state, no cache beyond an optional short-TTL default-branch map |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | `go 1.25.0` in `go.mod` (local toolchain 1.27.1) | Language | go-sdk and client-go both declare `go 1.25.0`; `CGO_ENABLED=0` build verified (10.6 MB with `-trimpath -ldflags "-s -w"` including client-go) [VERIFIED: local build] |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 (2026-09-04) | MCP server + in-memory/command client for tests | Locked D-12. `go list -m @latest` confirms it is current [VERIFIED: proxy.golang.org via `go list -m`] |
| `gitlab.com/gitlab-org/api/client-go/v2` | v2.64.0 (2026-09-05) | Typed GitLab REST client | Locked D-01. Current latest [VERIFIED: proxy.golang.org via `go list -m`] |
| Go stdlib `net/http`, `log/slog`, `context`, `encoding/base64`, `unicode/utf8`, `regexp` | bundled | Transport, logging, decode, redaction | No config/logging framework needed (CLAUDE.md) |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/hashicorp/go-retryablehttp` | v0.7.8 (transitive of client-go, but imported directly for the `CheckRetry`/`Backoff` types) | Type of `WithCustomRetry`/`WithCustomBackoff` callbacks | Import only the type; it is already in the module graph. Will become a direct requirement in `go.mod` [VERIFIED: compiled in spike] |
| `testing`, `net/http/httptest` | bundled | Fake GitLab | Always; record `r.RequestURI` (raw) for wire assertions |
| `mcp.NewInMemoryTransports()`, `mcp.CommandTransport`, `mcp.IOTransport` | in go-sdk v1.8.0 | In-process session, real-binary session, stdout-guarded server transport | Test tiers 2/3 and `main` [VERIFIED: executed in spike] |
| Python `mcp` | ==1.30.0 | Smoke client, identical to agent | `uv run` PEP 723 script (uv 0.12.5 present) [VERIFIED: executed] |
| `github.com/google/jsonschema-go` | v0.4.3 (transitive) | Schema inference from structs | Indirect only; use `jsonschema:"..."` tags |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `client-go/v2` | hand-rolled `net/http` | Rejected by D-01 (author decision). The spike shows the three concerns are controllable, so no reversal is proposed |
| `WithoutRetries()` + own loop | `WithCustomRetry` policy | `WithoutRetries()` also disables GET retries; custom policy keeps 429/5xx retry for reads |
| `WithOnlyIdempotentRetries()` | custom policy | It still retries PUT/DELETE and retries POST on 429 (source-verified); D-03 says never retry any write |
| `mcp.StdioTransport` | `mcp.IOTransport` | `StdioTransport` binds `os.Stdin/os.Stdout` directly, so a stray `fmt.Println` corrupts the stream; `IOTransport` lets us hand it the real stdout and repoint `os.Stdout` at stderr |

**Installation:**
```bash
go mod init gitlab-mcp            # module name is discretionary
go get github.com/modelcontextprotocol/go-sdk@v1.8.0
go get gitlab.com/gitlab-org/api/client-go/v2@v2.64.0
go get github.com/hashicorp/go-retryablehttp@v0.7.8   # direct import for callback types
go mod tidy
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp
```

**Version verification:** `go list -m -json <module>@latest` on 2026-09-24 returned go-sdk v1.8.0 (2026-09-04T08:08Z), client-go/v2 v2.64.0 (2026-09-05T20:30Z), go-retryablehttp v0.7.8 (2025-06-18), jsonschema-go v0.4.3 (2026-04-17). Resulting `go.mod` after `go mod tidy` declares `go 1.25.0` [VERIFIED: local].

## Package Legitimacy Audit

`slopcheck` (installed via pip) only checks pip/npm installs (`install`/`scan` subcommands, no Go-module mode), so it cannot rate Go modules. Modules below were resolved from the Go module proxy (checksum-verified by `go.sum`), are named by the locked decisions D-01/D-12 and by CLAUDE.md, and were compiled and executed in a spike. No `[SLOP]`/`[SUS]` signals apply; the author already approved these choices, so no `checkpoint:human-verify` install gates are needed.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| github.com/modelcontextprotocol/go-sdk | Go proxy | official SDK, v1.8.0 | n/a | github.com/modelcontextprotocol/go-sdk | n/a (Go) | Approved (locked D-12) |
| gitlab.com/gitlab-org/api/client-go/v2 | Go proxy | official GitLab client, v2.64.0 | n/a | gitlab.com/gitlab-org/api/client-go | n/a (Go) | Approved (locked D-01) |
| github.com/hashicorp/go-retryablehttp | Go proxy | 0.7.8 (2025-06) | n/a | github.com/hashicorp/go-retryablehttp | n/a (Go) | Approved (already a client-go dependency) |
| mcp (PyPI, test tooling only) | PyPI | 1.30.0, pinned by the agent | n/a | github.com/modelcontextprotocol/python-sdk | n/a | Approved (agent's own pin) |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
 Agent (Python mcp 1.30.x)                                   gitlab.com / fake GitLab (GITLAB_URL)
   | spawn gitlab-mcp.exe, env{GITLAB_TOKEN}                        ^
   | stdin  JSON-RPC lines            stdout (protocol ONLY)        | HTTPS + PRIVATE-TOKEN
   v                                  ^                             |
+--main.go------------------------------------------------------------------------------+
| 1 config.Load(getenv): token (TrimSpace) or stderr msg + exit 1; GITLAB_URL default    |
| 2 realOut := os.Stdout; os.Stdout = os.Stderr        <- stray Print* can no longer     |
| 3 logger = slog(stderr, Warn, redacting ReplaceAttr)    corrupt the protocol stream    |
| 4 glclient.New(cfg)  (NO network)                                                     |
| 5 server.New(client, logger) -> tools.Register  (5 tools)                              |
| 6 srv.Run(ctx, &mcp.IOTransport{os.Stdin, realOut})  -> returns nil on stdin EOF -> 0  |
+----------------------------------------------------------------------------------------+
        |  tools/call {name,args}
        v
  go-sdk: decode + validate against inferred schema  --invalid--> isError:true (readable text)
        |
        v
  tools.safe[T] wrapper:  defer recover()  |  ctx, cancel = WithTimeout(25s)
        |                                   |  err -> toToolError -> redact -> isError result
        v
  handler (thin): normalise inputs/defaults -> glclient calls -> format (one line/item, footer)
        |
        v
  glclient: client-go (custom CheckRetry GET-only, Backoff, RetryMax 2, no-op limiter)
            -> http.Client{Timeout 15s, dial 5s, TLS 5s, body cap 8 MiB} -> HTTP
        |<- 2xx: typed struct + Response.NextPage        | non-2xx: *gitlab.ErrorResponse / ErrNotFound
        v
  format: budget(15 000 runes) + footer  ->  CallToolResult{Content:[Text]} (isError false)
```

### Recommended Project Structure
```
gitlab-mcp/
├── cmd/gitlab-mcp/main.go        # composition root only (~60 lines)
├── internal/
│   ├── config/                   # Load(getenv) -> Config{Token, BaseURL}; validation; never prints the token
│   ├── logging/                  # New(w io.Writer, secret string) *slog.Logger; Redact(string) string
│   ├── glclient/                 # client-go construction + policy + normalisers + error classification
│   │   ├── client.go             # New(cfg): options (retry, backoff, limiter, transport, UA)
│   │   ├── retry.go              # CheckRetry, Backoff (GET/HEAD only)
│   │   ├── transport.go          # timeouts + response body cap
│   │   ├── normalize.go          # NormalizeProject, NormalizeRepoPath
│   │   └── errors.go             # Classify(err, resp) -> *Error{Kind, Status, RetryAfter, Detail}
│   ├── tools/
│   │   ├── register.go           # Register(server, c) ; one AddTool per tool
│   │   ├── safe.go               # generic wrapper: recover, deadline, error/redact mapping
│   │   ├── format.go             # Budget(), page footer, line renderers
│   │   ├── whoami.go  projects.go  tree.go  file.go
│   │   └── *_test.go             # in-memory MCP + fake GitLab
│   ├── server/server.go          # New(c, logger) *mcp.Server  (no Run here)
│   └── testutil/                 # fakegitlab.go (recorder + route table), session.go, binary.go
├── e2e/                          # stdio_test.go (real binary), purity_test.go, python_smoke_test.go (skips w/o uv)
├── scripts/smoke.py              # PEP 723 script, optional live mode when GITLAB_TOKEN set
├── go.mod / go.sum
```
`internal/glclient` is justified (CONTEXT left this open): D-02/D-03/D-04 policies, path normalisation and error classification are all client-go-facing concerns that Phases 2-4 reuse; tools keep using client-go's typed services and types directly, so it is a policy layer, not a re-wrapping of endpoints. The package name must differ from the imported `gitlab` package.

### Pattern 1: client-go construction with a fail-safe policy (verified)
**What:** Replace retry, backoff, limiter and HTTP client; keep per-call context via `gitlab.WithContext(ctx)`.
**When to use:** the only place a `*gitlab.Client` is created.
```go
// Source: spike (scratchpad/spike/gl/gl.go), executed against httptest with client-go v2.64.0
hc := &http.Client{
    Timeout: 15 * time.Second, // per attempt; the 25 s call deadline comes from ctx
    Transport: capBody(&http.Transport{
        Proxy:                 http.ProxyFromEnvironment,
        DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
        TLSHandshakeTimeout:   5 * time.Second,
        ResponseHeaderTimeout: 15 * time.Second,
        IdleConnTimeout:       30 * time.Second,
    }, 8<<20),
}
c, err := gitlab.NewClient(token,
    gitlab.WithBaseURL(baseURL),                               // client-go appends /api/v4
    gitlab.WithHTTPClient(hc),
    gitlab.WithCustomRetry(retryablehttp.CheckRetry(checkRetry)), // GET/HEAD only
    gitlab.WithCustomBackoff(backoff),                         // honours Retry-After (not RateLimit-Reset)
    gitlab.WithCustomRetryMax(2),
    gitlab.WithCustomLimiter(noLimiter{}),                     // Wait(ctx) error { return nil }
    gitlab.WithURLWarningLogger(logger),                       // default is slog.Default()
    gitlab.WithUserAgent("gitlab-mcp/0.1"),
)
```
```go
func checkRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
    if ctx.Err() != nil { return false, ctx.Err() }
    if err != nil { // resp == nil here; the method is in *url.Error.Op ("Get"/"Post"/"Put"...)
        var ue *url.Error
        if errors.As(err, &ue) && (ue.Op == "Get" || ue.Op == "Head") {
            var dns *net.DNSError
            return !(errors.As(err, &dns) && dns.IsNotFound), nil
        }
        return false, nil
    }
    if resp == nil || resp.Request == nil || !(resp.Request.Method == "GET" || resp.Request.Method == "HEAD") {
        return false, nil                                       // writes: never
    }
    switch {
    case resp.StatusCode == 429:
        ra := retryAfter(resp)                                  // parse seconds from Retry-After
        if ra > 5*time.Second { return false, nil }             // surface "retry in N s" instead of sleeping
        if dl, ok := ctx.Deadline(); ok && time.Until(dl) < ra+time.Second { return false, nil }
        return true, nil
    case resp.StatusCode >= 500 && resp.StatusCode != 501:
        return true, nil
    }
    return false, nil
}
func backoff(_, _ time.Duration, attempt int, resp *http.Response) time.Duration {
    if resp != nil && resp.StatusCode == 429 { if ra := retryAfter(resp); ra > 0 { return ra } ; return time.Second }
    return 500 * time.Millisecond
}
```
Budget arithmetic (D-02): `RetryMax=2` and each wait <=5 s gives <=10 s total waiting; the 25 s context bounds everything else. Verified outcomes: GET 503 -> 3 attempts in 0.9 s; GET 429 with `Retry-After: 1` -> 3 attempts in 2.0 s; POST 500 / POST 429 / PUT 503 / DELETE 503 -> exactly 1 attempt; GET 404 -> 1 attempt; connection refused GET -> 3 attempts (0.9 s), POST -> 1; a 1 s deadline against a slow server returned `context deadline exceeded` in 1.001 s and `errors.Is(err, context.DeadlineExceeded)` was true.

For defence in depth, also pass a "never retry" override on every write in Phases 2-3: `gitlab.WithRequestRetry(neverRetry)` exists as a per-request option (source-verified). The client-level policy is already GET-only, so this is belt and braces.

### Pattern 2: one wrapper for every handler (verified)
**What:** Generic `safe[T]` adds recover, deadline, error mapping. go-sdk v1.8.0 panics kill the process (verified: `panic: boom` with stack trace, process exited).
```go
// Source: spike (scratchpad/spike/inmem_test.go) - executed through in-memory MCP session
func safe[T any](timeout time.Duration, redact func(string) string,
    h func(ctx context.Context, in T) (string, error)) mcp.ToolHandlerFor[T, any] {
    return func(ctx context.Context, _ *mcp.CallToolRequest, in T) (res *mcp.CallToolResult, _ any, _ error) {
        defer func() {
            if r := recover(); r != nil {
                res = errResult(redact(fmt.Sprintf("internal error: %v", r)))
            }
        }()
        ctx, cancel := context.WithTimeout(ctx, timeout) // 25 s
        defer cancel()
        txt, err := h(ctx, in)
        if err != nil { return errResult(redact(toToolText(err))), nil, nil }
        return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: txt}}}, nil, nil
    }
}
```
Returning `(nil, nil, err)` also yields `isError:true` with the error text (verified over Python 1.30.0), but returning an explicit `*CallToolResult{IsError:true}` gives full control over wording and redaction.

### Pattern 3: stdout guard in `main` (verified)
```go
// Source: spike (scratchpad/spike/cmd/srv/main.go) - run under Python mcp 1.30.0 and a raw subprocess test
realOut := os.Stdout
os.Stdout = os.Stderr                           // any stray fmt.Println now goes to stderr
err := srv.Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: realOut})
// returns nil on stdin EOF -> exit 0 (measured: 2 ms after stdin close, empty stdout tail, LF-only lines, UTF-8 intact)
```
Note that `log.Print*` already defaults to stderr; `slog.SetDefault` should also point at stderr.

### Pattern 4: project and path normalisers (encoding is client-go's job)
**What:** Normalisers clean *semantics* and never percent-encode. client-go calls `PathEscape` on every string path argument (`ProjectID.forPath`, and `withPath` for plain strings such as file names), so pre-encoding double-encodes (`%2F` -> `%252F`).
```go
func NormalizeProject(s string) (string, error) {
    s = strings.TrimSpace(s)
    if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") { return "", errURLNotAllowed }
    if strings.Contains(s, "%") { if u, err := url.PathUnescape(s); err == nil { s = u } } // LLM sent group%2Fproj
    s = strings.Trim(s, "/")
    if s == "" { return "", errEmptyProject }
    return s, nil            // numeric "123" stays a string; client-go passes it through unchanged
}
func NormalizeRepoPath(p string) (string, error) { // for tree `path` (query) and file path (path segment)
    p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
    p = strings.TrimPrefix(strings.TrimPrefix(p, "./"), "/")
    for _, seg := range strings.Split(p, "/") { if seg == ".." { return "", errDotDot } }
    return p, nil
}
```
(A GitLab project path cannot contain `%`, so unescaping input that contains it is safe.)

**Verified wire encoding** (server saw these `RequestURI` values; client-go v2.64.0, project `g/p` unless noted):

| Input | Wire |
|-------|------|
| project `group/sub/my.proj` | `/api/v4/projects/group%2Fsub%2Fmy%2Eproj` |
| project `123` | `/api/v4/projects/123` |
| project `gr oup/проект` | `/projects/gr%20oup%2F%D0%BF%D1%80%D0%BE%D0%B5%D0%BA%D1%82` |
| project `a#b/c+d/e%f` | `/projects/a%23b%2Fc+d%2Fe%25f` (`+` stays literal) |
| file `src/main.go` | `/repository/files/src%2Fmain%2Ego` |
| file `dir with space/a#b.txt` | `.../files/dir%20with%20space%2Fa%23b%2Etxt` |
| file `a+b.txt` | `.../files/a+b%2Etxt` (`+` literal, see Open Question 1) |
| file `100%.txt` | `.../files/100%25%2Etxt` |
| file `файл/имя.go` | `.../files/%D1%84%D0%B0%D0%B9%D0%BB%2F%D0%B8%D0%BC%D1%8F%2Ego` |
| file `.gitignore` | `.../files/%2Egitignore` |
| file `a?b.txt` | `.../files/a%3Fb%2Etxt` |
| ref `feature/x y+z#1` (query) | `?ref=feature%2Fx+y%2Bz%231` (space is `+`, plus is `%2B`) |
| tree path `dir with space/ф+#%`, ref `feature/x`, page 2, per_page 20, recursive | `/projects/g%2Fsub%2Fp/repository/tree?page=2&path=dir+with+space%2F%D1%84%2B%23%25&per_page=20&recursive=true&ref=feature%2Fx` |

Request headers seen by the server: `Private-Token`, `Accept: application/json`, `Accept-Encoding: gzip`, `User-Agent`. The token is only in `Private-Token`.

### Pattern 5: output budget and footers
- Budget in **runes** (the agent cuts `len(str)` in Python, i.e. code points). Cut on a line boundary when possible; the footer is appended after the cut and is short (~150 chars) so 15 000 + footer stays under the agent's 20 000.
- Footer strings per D-07. Pagination: `[page N, per_page M — есть следующая страница: вызовите с page=N+1]` when `resp.NextPage > 0`, or (defensive) when `resp.NextLink != ""`; otherwise `[page N, per_page M — последняя страница]`. Never use `X-Total`.
- Clamp `per_page` to 1..100, default 20; `page` < 1 -> 1. Recursive tree: keep `per_page` default 20 but allow up to 100; the 15 000-rune budget is the real cap (say so in the footer, D-07).
- Error bodies are bounded too: cap detail text to ~300 runes and never echo HTML bodies (client-go turns a non-JSON body into `failed to parse unknown error format: <html>...`).

### Pattern 6: `get_file_contents` algorithm
1. `NormalizeProject`, `NormalizeRepoPath`; empty `ref` -> resolve default branch (D-11): `GetProject` -> `DefaultBranch`; if empty (`EmptyRepo`) return "репозиторий пуст". Optional process-lifetime cache keyed by normalised project with a short TTL (e.g. 60 s); fine either way, tests must be able to bypass it.
2. `RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(ref)}, gitlab.WithContext(ctx))`. `File{FileName, FilePath, Size, Encoding, Content, BlobID, LastCommitID, Ref}`. `ref` is required by GitLab for this endpoint (400 without it) [CITED: docs.gitlab.com/api/repository_files].
3. Decode base64 (`encoding == "base64"`; StdEncoding). Binary check (D-10): NUL byte in the first 8 000 bytes or `!utf8.Valid(prefix)` where the prefix is trimmed back to a rune boundary first (a cut inside a multi-byte rune must not count as invalid). Binary -> `файл бинарный, N байт, blob_id X` and stop.
4. Strip a UTF-8 BOM, normalise nothing else (CRLF stays; lines split on `\n`, trailing `\r` trimmed only for display counts).
5. `start_line`/`end_line` (plain `int`, `omitempty`, 1-based inclusive; 0 = unset): clamp to file; `end < start` -> error text. Apply the 15 000-rune budget to the selected slice; header line `path @ ref (ветка по умолчанию) — строки a-b из N, M байт`; truncation footer per D-09.
6. Big-file protection: the JSON endpoint returns base64 in one body. Cap the body in the transport (8 MiB verified pattern below) and map `errBodyTooLarge` to "файл слишком большой для чтения через API (>~6 МБ)". GitLab additionally rate-limits blobs over 10 MB to 5 req/min [CITED: docs.gitlab.com/api/repository_files].

```go
// Source: spike (scratchpad/spike/gl/limit_test.go) - 3 MiB body vs 1 MiB cap returned the sentinel through client-go untouched
var errBodyTooLarge = errors.New("response body too large")
type capBody struct{ io.ReadCloser; left int64 }
func (c *capBody) Read(p []byte) (int, error) {
    if c.left <= 0 { return 0, errBodyTooLarge }
    if int64(len(p)) > c.left { p = p[:c.left] }
    n, err := c.ReadCloser.Read(p); c.left -= int64(n); return n, err
}
type capRT struct{ rt http.RoundTripper; max int64 }
func (r capRT) RoundTrip(q *http.Request) (*http.Response, error) {
    resp, err := r.rt.RoundTrip(q)
    if err == nil { resp.Body = &capBody{ReadCloser: resp.Body, left: r.max} }
    return resp, err
}
```
(`errors.Is(err, errBodyTooLarge)` was true on the value returned by `RepositoryFiles.GetFile`.)

### Pattern 7: error classification (verified shapes)
client-go returns `(value, *gitlab.Response, error)`; on HTTP errors the `*Response` is non-nil, so `Retry-After` is read from `resp.Header`.

| Situation | What client-go returns (verified) | Map to |
|-----------|-----------------------------------|--------|
| 401 | `*gitlab.ErrorResponse`, text `GET <url>: 401 {message: 401 Unauthorized}` | "401: GitLab отклонил токен (недействителен, просрочен или отозван). Проверьте GITLAB_TOKEN." |
| 403 | `*ErrorResponse` (`{error: insufficient_scope}, ...` parsed) | "403: недостаточно прав: у токена нет нужного scope (`read_api`/`api`) или роль в проекте не позволяет" |
| 404 | **`gitlab.ErrNotFound` shared sentinel, message "404 Not Found", no body, no URL**; `errors.Is(err, gitlab.ErrNotFound)` true | tool-specific: "404: не найдено (проект, путь или ref). GitLab также отдаёт 404, если токен не видит приватный проект." Never mutate the sentinel |
| 429 | `*ErrorResponse`, message `failed to parse unknown error format: Retry later`, `resp.Header["Retry-After"]` present | "429: превышен лимит GitLab, повторите через N с" (N from `Retry-After`, default 60) |
| 400 with nested message | `{message: {base: [a, b]}}` flattened | GitLab text, capped |
| 5xx / HTML body | `... 502 failed to parse unknown error format: <html>...` | "5xx: ошибка сервера GitLab, повторите позже" - ignore the body |
| dial/DNS/TLS | `*url.Error`: `Get "https://...": dial tcp ...: connectex: ...` | "Не удалось связаться с gitlab.com: <cause>" (cause redacted, capped) |
| deadline | `context.DeadlineExceeded` via `errors.Is` | "Превышено время ожидания (25 с)" |
| client cancelled | `context.Canceled` | "Вызов отменён" |
| body cap | `errBodyTooLarge` | see Pattern 6 |
| JSON decode | `*json.SyntaxError`/`*json.UnmarshalTypeError` | "Неожиданный ответ GitLab" + log detail to stderr |

Detect with `errors.As(err, &*gitlab.ErrorResponse)` and `errors.Is(err, gitlab.ErrNotFound)`; for 404, `resp.StatusCode` is also available.

### Pattern 8: logging and redaction
- `logging.New(os.Stderr, token)` -> `slog.NewTextHandler` at `Warn` (or `Info` with `LOG_LEVEL`), `ReplaceAttr` that (a) redacts strings, (b) converts `error` values to redacted strings (`a.Value.Kind()==slog.KindAny` and `.Any().(error)`), and (c) also handles `slog.MessageKey`.
- `Redact(s)`: `strings.ReplaceAll(s, token, "[REDACTED]")` plus regexp `glpat-[A-Za-z0-9_\-]+` (and `glpat-` variants of other GitLab prefixes are not needed for a PAT-only server).
- Apply `Redact` in `safe()` to the final text as the last step, so a future error path cannot bypass logging redaction. Pass the same logger to `mcp.ServerOptions{Logger: ...}` (go-sdk logs session events at INFO only when a logger is given) and to `gitlab.WithURLWarningLogger`.
- Observed in the spike: no client-go/retryablehttp output at all unless a logger is configured (`retryablehttp.Client.Logger` is nil in client-go's struct literal). Never enable `WithRequestLogHook`/dumps.
- `TrimSpace` the token on load (a pasted trailing newline makes Go reject the header; the resulting error text was `net/http: invalid header field value for "Private-Token"` - it does NOT include the value [VERIFIED: executed]).

### Test Harness Design (QA-01, QA-02; `nyquist_validation` is false so no formal Validation Architecture section, but these are phase requirements)

| Tier | Where | How | Catches |
|------|-------|-----|---------|
| 1 Client unit | `internal/glclient/*_test.go` | `httptest` server + `glclient.New(server.URL, "glpat-TESTSECRET")`; assert `RequestURI` wire table, `Private-Token` header, attempt counts for GET 503/429, POST 500/429, PUT/DELETE 503, deadline, dial failure | Encoding, retry policy (D-02/D-03), error shapes |
| 2 Tool tests | `internal/tools/*_test.go` | `testutil.FakeGitLab` (route table + recorded requests) + `mcp.NewInMemoryTransports()` + `mcp.NewClient(...).Connect`; `ListTools` guard (names <=30, desc <=900, no `$ref/$defs/anyOf/oneOf/"null"`); `CallTool` for each tool incl. error statuses, truncation, binary, ranges, pagination footers, panic -> `isError`, token never in results | Schema shape, wiring, mapping, budget |
| 3 Stdio e2e | `e2e/` | `TestMain` builds the binary once into a temp dir (`go build`), starts a fake GitLab, `mcp.CommandTransport{Command: exec.Command(bin)}` with `cmd.Env = append(os.Environ(), "GITLAB_TOKEN=...", "GITLAB_URL="+fake.URL)`; asserts initialize + list + calls; asserts fake saw **0 requests** during initialize+tools/list (FND-01) | Real-process behaviour |
| 3b Purity | `e2e/purity_test.go` | raw `os/exec` pipes: write JSON lines, read stdout **in binary**; assert every line parses as JSON-RPC, no `\r`, then close stdin and assert exit code 0 within ~2 s and empty tail; include one failing call and a token-missing run (exit 1, message on stderr, empty stdout) | stray stdout writes, CRLF, EOF handling |
| 4 Python smoke | `scripts/smoke.py` (+ optional Go test that runs it via `uv run` and skips if `uv` is absent) | PEP 723 header, `stdio_client(StdioServerParameters(command=exe, env={"GITLAB_TOKEN":..., "GITLAB_URL":...}))`, `initialize` -> `list_tools` (assert protocol, names/desc/schema rules) -> `call_tool` calls incl. an error; if a real `GITLAB_TOKEN` is present in the parent env and no `--fake`, run live `whoami`/`list_projects` | Compatibility with the agent's exact SDK |

Windows notes: close every spawned process before `t.TempDir()` cleanup (locked `.exe`); do not use `-race` (no cgo); `python` on PATH is 3.13.15 (the `python3` alias is a Store stub, do not use it); prefer `uv run scripts/smoke.py`.

Fake server tips: implement one `http.HandlerFunc` that records `r.Method + " " + r.RequestURI` and dispatches by decoded/escaped path prefix; avoid `http.ServeMux` (path cleaning/redirects for odd segments) - the spike used a plain handler and every case in the wire table passed. Always assert on the raw `RequestURI`, not on `r.URL.Path` (which is already decoded).

### Anti-Patterns to Avoid
- **Relying on client-go defaults for retries** - 6 attempts on a failing POST, 11.8 s (verified).
- **`WithoutRetries()` as the D-03 solution** - kills read retries too.
- **Pre-encoding project or file paths** - client-go encodes once; a second pass yields `%252F`.
- **Building `*gitlab.Client` per call** - loses connection reuse; one shared client (it is goroutine-safe).
- **Returning raw `Project`/`File` structs as JSON** - D-06 requires compact text.
- **Using pointer fields in tool input structs** - schema becomes `["null", ...]` (STACK.md, verified earlier); use plain types + `omitempty`.
- **`jsonschema:"key=value ..."` descriptions** - panics at registration.
- **Echoing GitLab error bodies verbatim** - HTML 502 pages, and (elsewhere) URLs; classify instead.
- **Logging with `fmt.Print*`/default `log` before `os.Stdout` is redirected.**

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON-RPC/MCP lifecycle, schema inference, argument validation | custom protocol code | go-sdk `mcp.AddTool` + `IOTransport` | Verified: validation failures already return readable `isError` results |
| HTTP path percent-encoding for project IDs and file names | own encoder | client-go's `withPath` (calls `PathEscape`, `.`->`%2E`) | Verified wire table; also `gitlab.PathEscape`/`PathEscapeFileName` are exported if ever needed |
| Query encoding for `ref`, `path`, `page` | manual `url.Values` | client-go option structs (`url:` tags via go-querystring) | Verified: `+` and `#` correct |
| Pagination header parsing | manual `X-Next-Page` parsing | `Response.NextPage`, `Response.NextLink` | Already parsed by client-go |
| Base64 decode, UTF-8 validation, BOM | custom | `encoding/base64`, `unicode/utf8` | Stdlib |
| Retry engine | custom sleep loop | retryablehttp via `WithCustomRetry/Backoff/RetryMax` | Already inside client-go; only the policy is custom |
| In-process MCP test transport | pipes | `mcp.NewInMemoryTransports()` | Verified working |
| Process-level MCP client for e2e | manual JSON | `mcp.CommandTransport` | Verified; plus a separate raw-pipe purity test where raw bytes matter |

**Key insight:** the value of choosing client-go is typed services and correct encoding; every behaviour that touches safety (retry policy, timeouts, limiter, redaction, panic recovery, output size) is *configuration or a wrapper around it*, not something to reimplement.

## Common Pitfalls

### Pitfall 1: client-go's default retry policy retries writes (and long)
**What goes wrong:** Default `NewClient` = `RetryMax 5`, retries every 5xx for every method, retries 429 for any method; a failing POST took 11.8 s over 6 attempts in the spike. `WithOnlyIdempotentRetries` still retries PUT/DELETE and 429 on POST.
**Why it happens:** Defaults optimised for a generic library, not for a 30 s agent budget or non-idempotent commits.
**How to avoid:** Pattern 1 policy set at construction; a Tier-1 test per method (GET/POST/PUT/DELETE x 429/503) asserting attempt counts.
**Warning signs:** Duplicate comments/commits in later phases; tool calls hitting the agent's 30 s timeout.

### Pitfall 2: Built-in 429 backoff ignores `Retry-After` and a built-in limiter can block
**What goes wrong:** client-go's `rateLimitBackoff` waits until `RateLimit-Reset` (epoch) or an exponential fallback (up to minutes), and a default `rate.Limiter` is built from the first `RateLimit-Limit` header with `Wait(ctx)` blocking each call.
**How to avoid:** Custom `Backoff` (Retry-After, capped) and `WithCustomLimiter(noLimiter{})`. In the spike a server sending `RateLimit-Reset` 50 s ahead did not delay calls with the custom backoff.
**Warning signs:** Calls that sleep far beyond 25 s.

### Pitfall 3: go-sdk does not recover panics
**What goes wrong:** A nil-map or index panic in any handler (client-go's `populateLinkValues` indexes `strings.Split(...)[1]` on Link headers) terminates the whole server; the agent sees "connection closed".
**How to avoid:** `safe()` wrapper on every tool; a test with a deliberately panicking handler returning `isError`.

### Pitfall 4: `jsonschema` tag beginning with `word=` panics at registration
**What goes wrong:** `jsonschema:"path=file, relative"` -> panic `tag must not begin with 'WORD='` when `AddTool` runs (verified). Commas, quotes and Cyrillic are fine.
**How to avoid:** Descriptions must not start with `identifier=`; the in-memory `ListTools` test (which runs registration) catches it.

### Pitfall 5: LLM sends the numeric project ID as a JSON number
**What goes wrong:** With `Project string`, `{"project": 123}` is rejected before the handler: `validating /properties/project: type: 123 has type "integer", want "string"` as `isError` (verified). Same for `"page": "2"` (string for integer).
**How to avoid:** Describe the argument as `"project": numeric ID as a string ("12345") or full path group/subgroup/project`. The strict behaviour is acceptable (readable error, model retries). If the author later wants leniency, use the lower-level `Server.AddTool` with a hand-written flat schema and manual unmarshalling (not recommended for Phase 1; keeps the schema inference benefits).

### Pitfall 6: 404 loses its message in client-go
**What goes wrong:** All 404s become the shared `ErrNotFound` ("404 Not Found"), so "Project Not Found" vs "404 File Not Found" vs "Tree Not Found" is indistinguishable.
**How to avoid:** Pass a per-call `what` label into the mapper ("проект", "файл", "путь/ref в дереве"); empty repository tree also returns 404 - check `EmptyRepo`/`DefaultBranch` first.

### Pitfall 7: Non-JSON error bodies leak into messages
**What goes wrong:** `502 failed to parse unknown error format: <html>bad gateway</html>` (verified) and 429 `Retry later` become the message text.
**How to avoid:** Classify by status; use body text only for 400/409/422 (JSON messages), capped.

### Pitfall 8: Default branch handling
**What goes wrong:** Empty `ref` is legal for the tree endpoint (GitLab uses the default branch) but **required** for the file endpoint; D-11 forbids `HEAD` and wants the branch named in the output, so both tools must resolve it explicitly. New/empty repos have no default branch.
**How to avoid:** Shared `resolveRef(ctx, project, ref)`; handle `EmptyRepo`/empty `DefaultBranch` with a clear text; unit-test both branches.

### Pitfall 9: `simple=true` field coverage is unverified
**What goes wrong:** `list_projects` should use `simple=true` to shrink payloads; GitLab docs do not state that `default_branch` is included in the simple representation. If it is missing, lines print `()`.
**How to avoid:** Render the branch only when non-empty; verify live in the manual check (Open Question 2). Decoding into client-go's large `Project` struct is tolerant of missing fields.

### Pitfall 10: Windows process and file-lock friction in tests
**What goes wrong:** Temp-dir cleanup fails while the built `.exe` is still running; `go test -race` unavailable; `python3` alias is a Store stub.
**How to avoid:** `t.Cleanup` order (close session, wait for process, then remove); no `-race`; use `uv run` or `python`.

### Pitfall 11: Stdout purity regressions from dependencies
**What goes wrong:** Any dependency writing to `os.Stdout` corrupts the stream.
**How to avoid:** `os.Stdout = os.Stderr` after capturing the real stdout, plus the tier-3b raw-pipe test on every full session (including an error call). Verified that the guard catches direct `fmt.Println`.

### Pitfall 12: Agent environment differences
**What goes wrong:** The agent's `stdio_client` passes only a small env whitelist plus the configured `env`; `HTTPS_PROXY` from the developer shell is not inherited, and `GITLAB_TOKEN` must be in the agent config (PITFALLS.md #5).
**How to avoid:** The smoke script passes `env={...}` explicitly like the agent; README (Phase 4) documents it.

## Code Examples

### In-memory session helper and schema guard (executed)
```go
// Source: spike (scratchpad/spike/inmem_test.go)
ct, st := mcp.NewInMemoryTransports()
ss, _ := srv.Connect(ctx, st, nil)               // server side
cs, _ := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
lt, _ := cs.ListTools(ctx, nil)
b, _ := json.Marshal(lt.Tools[0].InputSchema)    // {"additionalProperties":false,"properties":{...},"required":["project"],"type":"object"}
res, _ := cs.CallTool(ctx, &mcp.CallToolParams{Name: "t", Arguments: map[string]any{"project": "x"}})
// res.IsError, res.Content[0].(*mcp.TextContent).Text
```
Schema guard: walk the marshalled schema; fail on keys `$ref`, `$defs`, `anyOf`, `oneOf`, on any string element `"null"` inside a `type` array, and on names >30 / descriptions >900.

### Real-binary session (executed)
```go
cmd := exec.Command(bin)
cmd.Env = append(os.Environ(), "GITLAB_TOKEN=glpat-TESTSECRET", "GITLAB_URL="+fake.URL)
cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "1"}, nil).
    Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
// ... calls ...
cs.Close()   // measured 1 ms against the spike server; the process exits on stdin EOF
```

### Raw purity check (Python form executed; Go form is the same with `os/exec` pipes)
```python
p = subprocess.Popen([exe], stdin=PIPE, stdout=PIPE, stderr=PIPE, env={**os.environ, "GITLAB_TOKEN": "glpat-x"})
# send initialize, initialized, tools/call; read lines with p.stdout.readline() (binary)
assert not line.endswith(b"\r\n")
p.stdin.close(); assert p.wait(timeout=5) == 0 and p.stdout.read() == b""
```
Observed: `crlf: False False`, `exit 0 after 0.002 s`, `remaining stdout bytes: 0`. Note: closing stdin immediately after piping requests (shell `printf | exe`) yields no responses because the server exits on EOF before answering - tests must keep stdin open until responses are read.

### Python smoke script skeleton (PEP 723; executed with `uv run`)
```python
# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.30.0"]
# ///
import asyncio, sys
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

async def main(exe, env):
    params = StdioServerParameters(command=exe, args=[], env=env)   # env like the agent's per-server config
    async with stdio_client(params) as (r, w):
        async with ClientSession(r, w) as s:
            init = await s.initialize()                             # protocolVersion 2025-11-25 observed
            tools = (await s.list_tools()).tools                    # assert name<=30, desc<=900, flat schema
            res = await s.call_tool("whoami", {})                   # res.isError / res.content[0].text
```
Behaviour observed against a go-sdk server through Python `mcp` 1.30.0: handshake negotiates `2025-11-25`; unknown/invalid arguments and handler errors arrive as `isError: true` with text; the server keeps serving afterwards; whole run 0.9 s including venv resolution.

### Tool input structs (flat, no pointers)
```go
type ListProjectsIn struct {
    Search        string `json:"search,omitempty" jsonschema:"filter by name or path substring"`
    IncludePublic bool   `json:"include_public,omitempty" jsonschema:"also include public projects you are not a member of; default false = only your projects"`
    Page          int    `json:"page,omitempty" jsonschema:"page number, default 1"`
    PerPage       int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}
type ProjectRefIn struct { // get_project
    Project string `json:"project" jsonschema:"numeric project ID as a string, or full path group/subgroup/project"`
}
type TreeIn struct {
    Project   string `json:"project" jsonschema:"numeric project ID as a string, or full path group/subgroup/project"`
    Path      string `json:"path,omitempty" jsonschema:"directory inside the repository, default repository root"`
    Ref       string `json:"ref,omitempty" jsonschema:"branch, tag or commit; default the project's default branch"`
    Recursive bool   `json:"recursive,omitempty" jsonschema:"list all nested entries; output is capped"`
    Page      int    `json:"page,omitempty" jsonschema:"page number, default 1"`
    PerPage   int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}
type FileIn struct {
    Project   string `json:"project" jsonschema:"numeric project ID as a string, or full path group/subgroup/project"`
    Path      string `json:"path" jsonschema:"file path inside the repository, for example src/main.go"`
    Ref       string `json:"ref,omitempty" jsonschema:"branch, tag or commit; default the project's default branch"`
    StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return, 1-based"`
    EndLine   int    `json:"end_line,omitempty" jsonschema:"last line to return, inclusive"`
}
// whoami: empty struct ({}) -> schema {"type":"object","additionalProperties":false}
```
Descriptions above intentionally avoid a leading `word=`. For `whoami` use `struct{}`: verified that go-sdk infers `{"additionalProperties":false,"type":"object"}` and that the call succeeds with both `arguments: null` and `arguments: {}`.

### client-go call shapes (compile-verified in v2.64.0)
```go
c.Projects.ListProjects(&gitlab.ListProjectsOptions{
    ListOptions: gitlab.ListOptions{Page: int64(p), PerPage: int64(pp)},
    Membership: gitlab.Ptr(true), Simple: gitlab.Ptr(true),
    OrderBy: gitlab.Ptr("last_activity_at"), Search: gitlab.Ptr(q),
}, gitlab.WithContext(ctx))                                   // ([]*Project, *Response, error)
c.Projects.GetProject(project, nil, gitlab.WithContext(ctx))  // *Project{ID, PathWithNamespace, DefaultBranch, EmptyRepo, Archived, Visibility, WebURL, Description, LastActivityAt}
c.Repositories.ListTree(project, &gitlab.ListTreeOptions{ListOptions: ..., Path: gitlab.Ptr(p), Ref: gitlab.Ptr(r), Recursive: gitlab.Ptr(true)}, gitlab.WithContext(ctx)) // []*TreeNode{ID,Name,Type,Path,Mode}
c.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(r)}, gitlab.WithContext(ctx))
c.Users.CurrentUser(gitlab.WithContext(ctx))                  // *User{ID, Username, Name, State, WebURL, Email, IsAdmin ...}
c.PersonalAccessTokens.GetSinglePersonalAccessToken(gitlab.WithContext(ctx)) // GET /personal_access_tokens/self: Scopes, ExpiresAt, Active (best-effort in whoami)
```
`ListOptions.Page`/`PerPage` are `int64` (non-pointer) in v2; all filter options are pointers (`gitlab.Ptr`). Always pass `gitlab.WithContext(ctx)` so the 25 s deadline and cancellation reach the HTTP layer.

`whoami` output suggestion: `id 123, @username (Имя), state active, https://gitlab.com/username`, then a second line `token: scopes [api], expires 2026-12-31` when `/personal_access_tokens/self` succeeds; if that second call fails, print `token: scopes unavailable` and still succeed. Do not print email.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `xanzy/go-gitlab` | `gitlab.com/gitlab-org/api/client-go/v2` | last xanzy tag v0.115.0 (2024-12) | Locked D-01 |
| `mcp.StdioTransport` always | `mcp.IOTransport` when you want to own stdout | present in go-sdk v1.8.0 | Enables the stdout guard |
| client-go retries by default for all methods | `WithOnlyIdempotentRetries()` added; "expected to become the default in the next major release" (source comment) | v2.x | Still not sufficient for D-03; custom policy needed |
| Tool handler panics | Not recovered by go-sdk v1.8.0 | - | Wrapper needed |

**Deprecated/outdated:** `NewBasicAuthClient`, `NewOAuthClient` (client-go) - not used; PAT via `NewClient(token)`.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Literal `+` in a URL *path* segment (`a+b%2Etxt`, project paths) is treated as a plus, not a space, by gitlab.com/Rails | Pattern 4, Open Q1 | Files or projects containing `+` return 404; needs a live check (Phase 4 or manual now). GitLab project paths cannot contain `+`; only repo file names can |
| A2 | `GET /projects?simple=true` includes `default_branch` | Pitfall 9 | `list_projects` lines lack the branch; degrade by omitting it |
| A3 | Offset-paginated `GET .../repository/tree` returns `X-Next-Page` (and `Link rel=next`) on gitlab.com | Pattern 5 | `list_repository_tree` would never show the next-page hint; the defensive `NextLink` check and a "len == per_page" note mitigate |
| A4 | `GET /personal_access_tokens/self` works for any valid PAT on gitlab.com without extra scope | whoami | Second call fails (403/404): whoami must treat it as best-effort (designed so) |
| A5 | Rune budget is the right unit for the agent's 20 000 cut (Python `len(str)`) | Pattern 5 | Slightly different cut; 15 000 leaves 5 000 headroom |
| A6 | Not verified: the `Retry-After` header on gitlab.com 429s is integer seconds | Pattern 1 | Parse failure -> default 1 s / surface; HTTP-date form would need parsing (unlikely) |

**Tagged claims in this document:** everything not marked `[VERIFIED]`/`[CITED]` above was checked by executing code in the spike or reading client-go v2.64.0 / go-sdk v1.8.0 source in the local module cache, except the six items above.

## Open Questions (RESOLVED)

1. **`+` and other path characters on real gitlab.com**
   - What we know: client-go leaves `+` literal in path segments; `%2B` would be the strictest encoding; Rack treats `+` in paths as a plus.
   - What's unclear: whether gitlab.com's edge (Cloudflare/Workhorse) behaves identically for `repository/files/a+b.txt`.
   - Recommendation: keep client-go's behaviour (it is what every Go GitLab tool sends); add a file named `a+b.txt` to the live smoke set (author's test project) and record the result. No code change planned unless it fails.
   - RESOLVED: keep client-go's path encoding unchanged in Phase 1; the live `+` check moves to Phase 4 (QA-03).
2. **`simple=true` and `default_branch` (A2), tree `X-Next-Page` (A3), PAT self (A4)**
   - Recommendation: the Python smoke script's optional live mode should print the first `list_projects` page, one tree page with `per_page=1`, and `whoami`, so the author can eyeball all three once with a real token. Phase 1 acceptance stays hermetic.
   - RESOLVED: optional live mode in `scripts/smoke.py` (Plan 06 Task 2) prints these; Phase 1 acceptance is hermetic only.
3. **Should numeric project IDs also be accepted as JSON numbers?**
   - What we know: strict schema rejects `123` with a readable error (verified).
   - Recommendation: keep strict for Phase 1 (schema-inferred, flat); revisit only if the agent's model repeatedly sends numbers.
   - RESOLVED: strict string schema; a JSON number is rejected with a readable isError (Plan 04 test).
4. **Tool annotations** (`ReadOnlyHint: true` on the five tools).
   - REQUIREMENTS lists UX-01 as v2, CLAUDE.md calls them cheap. Recommendation: add `Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}` on these read tools (one line, no schema impact); planner may drop it.
   - RESOLVED: `ReadOnlyHint: true` is added on all five read tools (Plans 01, 04, 05, 06); this partially lands v2 UX-01 early.
5. **Default-branch cache**
   - Recommendation: small `sync.Mutex` map with 60 s TTL; skip if the planner prefers zero state (cost: one extra call per file read when `ref` is empty).
   - RESOLVED: zero state, no cache; `resolveRef` calls GetProject when `ref` is empty.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build/tests | yes | 1.27.1 windows/amd64 | - |
| cgo/gcc | `-race` only | no | - | `CGO_ENABLED=0`, no `-race` (documented) |
| uv | Python smoke (`uv run` PEP 723) | yes | 0.12.5 | `python` + pre-installed `mcp` |
| Python | smoke/purity script | yes | 3.13.15 (`python`; `python3` is a Store stub) | uv-managed Python |
| Python `mcp` 1.30.0 | smoke client | yes (system site-packages and via `uv run --with`) | 1.30.0 | - |
| gitlab.com + real PAT | optional live check | not needed for acceptance | - | hermetic fake; live check is Phase 4 |
| Node/npx (MCP Inspector) | optional dev poking | not required | - | Python smoke is primary |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** cgo (no `-race`).

## Security Domain

`security_enforcement` is absent from `.planning/config.json` (treated as enabled).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (PAT only) | `GITLAB_TOKEN` from env, trimmed, header-only (`PRIVATE-TOKEN` set by client-go), fail fast when absent |
| V3 Session Management | no | stdio single-user process, no sessions |
| V4 Access Control | delegated | Authorisation is the token's scopes/roles on GitLab; no server-side ACL by design (PROJECT.md) |
| V5 Input Validation | yes | go-sdk schema validation (types, `additionalProperties:false`) + normalisers (reject URLs as project, reject `..`, clamp `page/per_page`, clamp line ranges) |
| V6 Cryptography | no | TLS via Go stdlib to gitlab.com; no custom crypto |
| V7 Error handling/logging | yes | redacting logger, redaction on final tool text, capped error text, no header/config dumps |
| V9 Communications | yes | `GITLAB_URL` accepted only as `https://` (or `http://` to loopback for tests); never derive the host from tool arguments (prevents token exfiltration to attacker-chosen hosts) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token disclosure via logs/errors/tool output | Information disclosure | `Redact` (exact token + `glpat-[A-Za-z0-9_-]+`) in slog `ReplaceAttr` and in `safe()`; test with fake `glpat-TESTSECRET` under 401/dial/deadline/panic and grep stdout, stderr, results |
| Token sent to attacker host via a URL "project" argument | Information disclosure | Reject `http(s)://` project refs; base URL only from env; client-go refuses requests to other hosts (source: `NewRequestToURL` host check) |
| Prompt injection via repository content returned to the model | Tampering | Return content as data with header line; Phase 1 tools are read-only; no auto-chaining |
| Path traversal / odd paths in `path` | Tampering | Reject `..`, strip leading `/`, `./`, convert `\` |
| Resource exhaustion (huge files/trees) | Denial of service | 8 MiB body cap, 15 000-rune output budget, per_page clamp, 25 s deadline, http timeouts |
| Handler panic kills the server | Denial of service | `safe()` recover |
| Malformed/newline token | Tampering | `TrimSpace`; Go's header validation error does not echo the value (verified) |

## Sources

### Primary (HIGH confidence)
- Local spike, executed on this machine (Windows 11, Go 1.27.1, uv 0.12.5, Python `mcp` 1.30.0): client-go v2.64.0 retry/backoff/limiter/timeout/body-cap/error-shape/wire-path tests; go-sdk v1.8.0 handshake, `IOTransport` stdout guard, `CommandTransport`, in-memory session, panic and tag-panic behaviour; binary size 10.6 MB with client-go.
- client-go v2.64.0 source in the module cache: `gitlab.go` (`NewAuthSourceClient`, `retryHTTPCheck`, `retryHTTPBackoff`, `rateLimitBackoff`, `configureLimiter`, `NewRequest`, `Do`, `CheckResponse`, `ErrorResponse`, `populatePageValues`), `client_options.go`, `request_handler.go` (`withPath`, `ProjectID.forPath`), `request_options.go` (`WithContext`, `WithRequestRetry`), `repository_files.go`, `repositories.go`, `projects.go`, `users.go`, `personal_access_tokens.go`.
- go-sdk v1.8.0 via `go doc` (`IOTransport`, `StdioTransport`, `CommandTransport`, `ServerOptions`) and executed behaviour.
- proxy.golang.org via `go list -m -json ...@latest` (versions/dates).
- https://docs.gitlab.com/api/repositories/ - tree params; `ref` optional, default branch used; `per_page` default 20 [fetched 2026-09-24]
- https://docs.gitlab.com/api/repository_files/ - `ref` required, response fields, HEAD headers, URL-encoded `file_path`, 10 MB blob rate limit [fetched 2026-09-24]
- https://docs.gitlab.com/api/personal_access_tokens/ - `GET /personal_access_tokens/self` [fetched 2026-09-24]
- https://docs.gitlab.com/api/projects/ - `membership`, `simple`, `order_by` default `created_at`, `empty_repo`, project path as ID [fetched 2026-09-24]
- Project docs: `.planning/research/ARCHITECTURE.md`, `PITFALLS.md`, CLAUDE.md (agent constraints: 10 s connect, 30 s call, 20 000 char cut, name/description limits, env whitelist).

### Secondary (MEDIUM confidence)
- Web search on `+` in `file_path`: no authoritative statement found (hence Open Question 1).

### Tertiary (LOW confidence)
- None relied upon.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - versions confirmed on the module proxy; both libraries compiled and executed.
- Architecture: HIGH - every pattern (retry policy, safe wrapper, IOTransport guard, body cap, wire encoding) was executed; layering follows the existing ARCHITECTURE.md.
- Pitfalls: HIGH for the client-go/go-sdk items (reproduced); MEDIUM for live-GitLab items (A1-A4), flagged and routed to a cheap live check.

**Research date:** 2026-09-24
**Valid until:** 2026-10-24 (client-go publishes majors roughly every 6 months and minors weekly; the pinned `v2.64.0` makes this stable for the phase)
