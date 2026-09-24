# Phase 3: merge-requests - Pattern Map

**Mapped:** 2026-09-24
**Files analyzed:** 21 (5 new production files, 3 modified production files, 7 new test files, 1 golden fixture, 4 other modified files/docs)
**Analogs found:** 21 / 21 (all in-repo, Phase 1-2 code; no external analog needed)

Module path is `gitlab-mcp`. Imports: `gitlab "gitlab.com/gitlab-org/api/client-go/v2"`, `gitlab-mcp/internal/glclient`, `github.com/modelcontextprotocol/go-sdk/mcp` (register.go only). All line numbers refer to the current tree (Phase 2 complete). Tool count goes 12 -> 20.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/tools/mr.go` (list_merge_requests, get_merge_request, create_merge_request, update_merge_request, In structs, line/header renderers, draft helpers) | tool handler | request-response; paginated list (list), header render (get), POST/PUT write (create/update) | `internal/tools/commits.go` (`listCommits`, `getCommit`, `commitHeader`) + `internal/tools/branch.go` (`createBranch`) | exact (read), role-match (writes) |
| `internal/tools/mr_status.go` (`mergeStatusAdvice` table, `statusAdvice`, `stateAdvice`) | utility (lookup table + formatter) | transform | `internal/tools/errors.go` `writeRules` table + `emptyPatchKind` in `diff.go` | role-match |
| `internal/tools/mr_diff.go` (get_merge_request_diffs, `mrDiffToDiffFile` adapter) | tool handler | request-response, paginated, bounded body | `internal/tools/commits.go` `getCommit` (lines 150-198) + `diff.go` `renderDiffFiles` | exact |
| `internal/tools/mr_notes.go` (list_merge_request_notes, create_merge_request_note) | tool handler | paginated list (read), POST write | `internal/tools/commits.go` `listCommits` (list) + `branch.go` `createBranch` (write) | role-match |
| `internal/tools/mr_merge.go` (merge_merge_request: GET pre-check + single PUT) | tool handler | request-response, pre-read then non-retried PUT | `internal/tools/write.go` `createOrUpdateFile` (lines 210-272: prior GET, guard, then one POST via `withWrite`) | role-match (closest "read then single write") |
| `internal/tools/errors.go` (modify: `opCreateMR/opUpdateMR/opMergeMR/opMRNote`, `writeRule.status`, empty-`substrings` matching, MR rules, 999.2 unknown-outcome extension) | utility (error mapper) | transform | itself (lines 41-55, 124-225) | exact (extend) |
| `internal/tools/register.go` (modify: +8 `AddTool`) | config | registration | itself (lines 64-81 for write tools) | exact |
| `internal/tools/testdata/tools_list.golden.json` (regenerate) | fixture | snapshot | itself; regenerate with `go test ./internal/tools -run TestToolsListGolden -update` | exact |
| `internal/tools/mr_test.go`, `mr_status_test.go`, `mr_diff_test.go`, `mr_notes_test.go`, `mr_merge_test.go` | test | request-response over fake | `internal/tools/branch_test.go` (write tests) + `commits_test.go` (get_commit tests, lines 206-322) | exact |
| `internal/tools/errors_test.go` (modify: MR cases, 999.2 kinds) | test | table-driven | itself (`TestToToolText`, lines 17-154) | exact (extend) |
| `internal/tools/schema_test.go` | test | schema guard | itself; NO change needed, `TestToolSchemas` iterates every registered tool | exact (verify only) |
| `scripts/smoke.py` (modify: `EXPECTED_TOOLS` 12 -> 20, new hermetic calls, optional 999.5 guard) | test tool (E2E client) | request-response | itself (lines 36-49, `call` helper line 143, `if base_url:` line 167) | exact (extend) |
| `e2e/python_smoke_test.go` (modify: fake routes for MR endpoints) | test | E2E | itself (lines 23-77 `fake.JSON` block) | exact (extend) |
| `e2e/stdio_test.go` (modify: `wantNames` line 197 lists all 12 tools) | test | E2E | itself | exact (MUST update, hard-coded list) |
| `.planning/ROADMAP.md` (close 999.2) | doc | n/a | existing backlog entries | n/a |

## Pattern Assignments

### `internal/tools/mr.go` - list_merge_requests (paginated list)

**Analog:** `internal/tools/commits.go` `listCommits` (lines 79-148) and `branch.go` `listBranches` (lines 47-84).

**Imports pattern** (`commits.go` lines 3-14; `branch.go` lines 3-12 is the minimal variant):
```go
import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)
```
Add `regexp` for the draft prefix and the `!N` extraction; three groups (stdlib, third-party, module-local).

**Description const + In struct** (`branch.go` lines 14-20, 33-38; `commits.go` lines 44-54). Russian description const `<tool>Description` (<=900 runes), English `jsonschema` tags that do NOT start with `word=`, plain fields, `omitempty` for optional, `page`/`per_page` copied verbatim:
```go
type BranchesIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Search  string `json:"search,omitempty" jsonschema:"substring of the branch name to filter by"`
	Page    int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}
```
For MR: add `State`, `SourceBranch`, `TargetBranch`, `AuthorUsername`, `Search` (all plain `string`, `omitempty`). `iid` for other tools is `int` with `json:"iid"` (client-go wants `int64`: `int64(in.IID)`); validate `> 0` before any request.

**Core handler pattern** (`branch.go` lines 48-83): normalise -> ClampPaging -> typed opts, optional filter only when non-empty -> client call with `gitlab.WithContext(ctx)` -> `withSubject` -> header + lines -> `composeList`:
```go
project, err := glclient.NormalizeProject(in.Project)
if err != nil {
	return "", err
}
page, perPage := ClampPaging(in.Page, in.PerPage)
opts := &gitlab.ListBranchesOptions{
	ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
}
search := strings.TrimSpace(in.Search)
if search != "" {
	opts.Search = gitlab.Ptr(search)
}
branches, resp, err := d.GL.Branches.ListBranches(project, opts, gitlab.WithContext(ctx))
if err != nil {
	return "", withSubject("проект", err)
}
header := "ветки " + project
if len(branches) == 0 {
	return header + "\nветки не найдены", nil
}
lines := make([]string, 0, len(branches)+1)
lines = append(lines, header)
for _, b := range branches { lines = append(lines, branchLine(b)) }
return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
```
MR-specific deviations (from RESEARCH F3): `State` must ALWAYS be sent (`gitlab.Ptr("opened")` when empty, since GitLab defaults to `all`); validate `state` against `{opened, closed, merged, all}` in the handler with a Russian error before any request; put the effective state in the header (`MR проекта g/p, state=opened`). Empty list text `"MR не найдены"` in the style of `"коммиты не найдены"` (`commits.go` line 128).

**Per-line renderer** (`commits.go` lines 140-148, `branch.go` lines 88-111): a separate `mrLine(*gitlab.BasicMergeRequest) string` built with `strings.Join(parts, " ")`; one line per MR `!iid state [draft] title source→target @author`; long text via `oneLine(title, N)` with a named `const` cap (like `commitTitleRunes = 120`, `branchTitleRunes = 100`). Marker style is bracketed lowercase (`[default]`, `[protected]`), so use `[draft]`.

---

### `internal/tools/mr.go` - get_merge_request (header + status)

**Analog:** `internal/tools/commits.go` `commitHeader` (lines 200-234).

Build the header as `[]string` joined with `"\n"`; long free text through `Budget(text, cap)` with an explicit cut marker line, exactly as the commit message is handled:
```go
msg, cut := Budget(strings.TrimSpace(c.Message), commitMessageRunes)
lines = append(lines, "сообщение:", msg)
if cut {
	lines = append(lines, "[сообщение обрезано]")
}
if c.WebURL != "" {
	lines = append(lines, c.WebURL)
}
```
Use for the MR description (`mrDescriptionRunes = 1500`, marker `[описание обрезано]`). Add `web_url`, conflict flag, head-pipeline (`HeadPipeline` may be nil: print `pipeline: нет`, RESEARCH F7), `ChangesCount` (a string: empty -> `число файлов: ещё не известно`), and `detailed_merge_status` via `statusAdvice(mr)` from `mr_status.go`. Exactly ONE `GetMergeRequest` request (D-05, no polling). 404: `withSubject("MR (iid) или проект", err)` (RESEARCH F10).

Single-object output: pass through `Budget(out, OutputBudget)` + `TruncatedFooter` as at `commits.go` lines 187-193 (do not add a page footer; only lists get one).

---

### `internal/tools/mr.go` - create_merge_request (POST with pre-reads)

**Analog:** `internal/tools/branch.go` `createBranch` (lines 113-145); pre-check analog `compare.go` `fetchCompare` (lines 46-60, reused unchanged).

**Guards -> resolveRef -> single write -> withWrite** (`branch.go` lines 116-136):
```go
project, err := glclient.NormalizeProject(in.Project)
if err != nil {
	return "", err
}
branch := strings.TrimSpace(in.Branch)
if branch == "" {
	return "", errors.New("не указано имя ветки")
}
ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
if err != nil {
	return "", err
}
b, _, err := d.GL.Branches.CreateBranch(project, &gitlab.CreateBranchOptions{...}, gitlab.WithContext(ctx))
if err != nil {
	return "", withWrite(opCreateBranch, "проект или ветка", err)
}
```
Adapt: guards on `source_branch` and `title` (plain `errors.New`, Russian, zero requests; `toToolText` passes `KindOther` detail through, see `errors_test.go` line 53). `target, isDefault, err := resolveRef(ctx, d, project, in.TargetBranch)`; reject `source == target` locally; then `res, err := fetchCompare(ctx, d, project, target, source)` on error `withSubject("ветка или проект", err)`; if `!res.CompareTimeout && len(res.Commits) == 0` return the "нет изменений между ветками ... commit_files" error (RESEARCH F4/Pattern 5). Then ONE POST, error via `withWrite(opCreateMR, "проект или ветка", err)`. Success text in the `createBranch` style (`branch.go` lines 142-143): `MR !iid создан: source→target`, `refLabel(target, isDefault)`, `web_url`, plus the mandatory "повторите get_merge_request перед merge" hint (D-05).

**409 duplicate** (D-09): classify before delegating to `withWrite`: `e := glclient.Classify(err)`; `e.Status == 409` and detail contains `another open merge request already exists` -> regex `!(\d+)` on `e.Detail` (`regexp.MustCompile("!(\\d+)")`); if absent, ONE safe GET `ListProjectMergeRequests(State=opened, SourceBranch=source, PerPage=1)`. Return `errors.New("открытый MR уже есть: !N (<web_url>)")`; never turn it into success.

**Draft via title prefix** (RESEARCH Pattern 3; no in-repo analog): package-level `regexp.MustCompile` + `withDraftPrefix(title)` exactly as in RESEARCH lines 184-191; `draft=true` sends `Title: gitlab.Ptr(withDraftPrefix(title))`. Show `mr.Draft` from the response, not the intention.

---

### `internal/tools/mr.go` - update_merge_request (PUT, optional-field semantics)

**Analog:** optional-field handling in `commits.go` lines 105-117 (`if x != "" { opts.X = gitlab.Ptr(x) }`) and the guard style in `write.go` `validateCommitInput` (lines 138-167).

```go
opts := &gitlab.ListCommitsOptions{...}
if path != "" {
	opts.Path = gitlab.Ptr(path)
}
if author := strings.TrimSpace(in.Author); author != "" {
	opts.Author = gitlab.Ptr(author)
}
```
Apply per field (`title`, `description`, `target_branch`, `state_event`): empty = not sent. "Nothing passed" -> `errors.New(...)` BEFORE any request. Validate `state_event` in the handler (`close`/`reopen`) with a Russian error (not a schema `enum`, per Phase 2 convention). `draft=true` without `title` costs ONE prior `GetMergeRequest` (read), then `withDraftPrefix(current.Title)`; `UpdateMergeRequestOptions` has no `Draft` field. Error: `withWrite(opUpdateMR, "MR (iid) или проект", err)`. Document the "empty = not changed / cannot clear" limitation in the description const (D-10).

---

### `internal/tools/mr_status.go` - status advice table

**Analog:** `errors.go` `writeRules` (lines 124-187) for the "table + lookup function" shape, and `diff.go` `emptyPatchKind` (lines 74-91) for a switch returning Russian reason text.

Structure (RESEARCH lines 309-335): `var mergeStatusAdvice = map[string]string{...}`; `func statusAdvice(mr *gitlab.MergeRequest) string` returning `"detailed_merge_status: <raw> — <advice>"`; unknown value -> raw + generic hint; empty -> "статус слияния не вернулся; повторите get_merge_request"; `func stateAdvice(mr)` for `merged`/`closed`/`locked` checked BEFORE the status (a merged MR reports `not_open`). Include the `draft_status` advice "update_merge_request с title без префикса «Draft:»" (RESEARCH O2). Keep this table pure so it is unit-testable without HTTP (`mr_status_test.go`, table-driven like `TestToToolText`).

---

### `internal/tools/mr_diff.go` - get_merge_request_diffs

**Analog:** `commits.go` `getCommit` (lines 150-198), verbatim structure. `renderDiffFiles` (`diff.go` lines 137-178) and `emptyPatchKind` are reused unchanged.

**Core flow to copy** (`commits.go` lines 161-196): ClampPaging -> read object (404 wording) -> read diff page -> header with file count -> `renderDiffFiles` with budget minus header minus `diffBodySlack` -> `Budget` + `TruncatedFooter` -> `PageFooter` from the DIFF call's `resp`:
```go
const subject = "коммит (sha, ветка или тег)"
c, _, err := d.GL.Commits.GetCommit(project, sha, nil, gitlab.WithContext(ctx))
if err != nil {
	return "", withSubject(subject, err)
}
files, resp, err := fetchCommitDiff(ctx, d, project, sha, page, perPage)
if err != nil {
	return "", withSubject(subject, err)
}
header := commitHeader(c, len(files))
body := "изменений в файлах нет (коммит без diff или страница за пределами списка)"
if len(files) > 0 {
	oldRef := c.ID
	if len(c.ParentIDs) > 0 {
		oldRef = c.ParentIDs[0]
	}
	budget := OutputBudget - utf8.RuneCountInString(header) - diffBodySlack
	if budget < 0 {
		budget = 0
	}
	body = renderDiffFiles(files, c.ID, oldRef, budget)
}
out, truncated := Budget(header+"\n"+body, OutputBudget)
var sb strings.Builder
sb.WriteString(out)
if truncated {
	sb.WriteString("\n")
	sb.WriteString(TruncatedFooter)
}
sb.WriteString("\n")
sb.WriteString(PageFooter(page, perPage, resp))
```
MR adaptation: call 1 `GetMergeRequest` (subject `"MR (iid) или проект"`), call 2 typed `d.GL.MergeRequests.ListMergeRequestDiffs(project, int64(iid), &gitlab.ListMergeRequestDiffsOptions{ListOptions: ...}, gitlab.WithContext(ctx))` (the typed struct already carries `Collapsed`/`TooLarge`, so the raw `NewRequest`/`Do` trick of `fetchCommitDiff`, `diff.go` lines 57-72, is NOT needed; fall back to it only if a field is missing). Adapter (RESEARCH lines 168-174) copies into `diffFile`. Refs: `newRef = mr.SHA` (fallback `mr.SourceBranch`), `oldRef = mr.DiffRefs.BaseSha` (fallback `mr.TargetBranch`); `DiffRefs` may be zero/nil right after creation (F7). Overflow line derived from `strings.HasSuffix(mr.ChangesCount, "+")` (there is no `overflow` field on `/diffs`, F1): separate header line "часть файлов не вернулась: в MR больше 1000 файлов (changes_count=1000+)". Empty page: "diff ещё готовится" when `ChangesCount == ""` vs "изменений в файлах нет ..." otherwise.

---

### `internal/tools/mr_notes.go` - list_merge_request_notes / create_merge_request_note

**Analog (list):** `commits.go` `listCommits` + `commitLine` (lines 140-148); `composeList` for footer. **Analog (create):** `branch.go` `createBranch`.

List: `d.GL.Notes.ListMergeRequestNotes(project, int64(iid), &gitlab.ListMergeRequestNotesOptions{ListOptions: ...}, gitlab.WithContext(ctx))`, no `OrderBy`/`Sort` (server default is `created_at desc`, D-07). Renderer per note: system -> `"[system] " + oneLine(body, 120)`; user -> header `#id @username date` then body via `Budget(body, 1000)` + `[заметка обрезана]` marker (same marker style as `commitHeader`). Date layout: reuse `commitDateLayout = "2006-01-02"` (`commits.go` line 41), guard `CreatedAt == nil` with `"-"` exactly like `commitLine`. Empty list: `header + "\nзаметок нет"`. Result via `composeList(...)`. Mention in the description const that `per_page` should be lowered for long threads (RESEARCH P13).

Create: guard `iid > 0` and `strings.TrimSpace(body) == ""` -> plain Russian `errors.New` with zero requests; `d.GL.Notes.CreateMergeRequestNote(project, int64(iid), &gitlab.CreateMergeRequestNoteOptions{Body: gitlab.Ptr(body)}, gitlab.WithContext(ctx))`; error `withWrite(opMRNote, "MR (iid) или проект", err)`. Print `note.ID` only if `> 0` (RESEARCH F8) plus MR `web_url`. Note: `Note` response has no MR URL; if `web_url` is required in the reply, either build it from the note or accept a bare id (planner decision; D-11 asks for id + MR web_url, so a GET of the MR is NOT needed only if `web_url` can be derived; otherwise state only the id and iid).

---

### `internal/tools/mr_merge.go` - merge_merge_request

**Analog:** `write.go` `createOrUpdateFile` (lines 210-272): normalise, guards, prior GET whose failure is `withSubject` (NOT `withWrite`), then exactly one write via `withWrite`, success text assembled as `[]string` joined with `"\n"`:
```go
f, _, err := d.GL.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(branch)}, gitlab.WithContext(ctx))
switch {
case err == nil:
	...
case glclient.Classify(err).Kind == glclient.KindNotFound:
	fa.Action = actCreate
default:
	return "", withSubject("файл", err)
}
commit, err := commitCore(ctx, d, project, branch, in.CommitMessage, []fileAction{fa})
if err != nil {
	return "", err
}
```
MR adaptation: RESEARCH Pattern 4 (lines 197-210). Blocking pre-check outcomes (`state != "opened"`, `DetailedMergeStatus != "mergeable"`) return a PLAIN `errors.New(stateAdvice/statusAdvice ...)` so `toToolText` passes the Russian text through (never wrap in `withWrite`, and send NO PUT). Optional PUT fields only when set (`if in.Squash { opts.Squash = gitlab.Ptr(true) }`; `ShouldRemoveSourceBranch` is the client-go name for `remove_source_branch`; `MergeCommitMessage` only when non-blank). Do NOT send `SHA` (MRX-03 deferred). Success text: iid, state, `MergeCommitSHA` (fallback `SquashCommitSHA`, then `SHA`, labelled), `remove_source_branch: запрошено` (never claim "удалена"), `web_url`. If `merged.State != "merged"` say so and point to `get_merge_request`.

---

### `internal/tools/errors.go` (modify) - MR write rules + 999.2

**Analog:** itself.

**Op keys** (lines 41-45): add alongside
```go
const (
	opCreateBranch = "create_branch"
	opCommit       = "commit"
)
```
the four new keys `opCreateMR`, `opUpdateMR`, `opMergeMR`, `opMRNote` (RESEARCH lines 341-346).

**`writeRule` + matcher** (lines 127-135, 196-212). Current matching requires at least one substring hit (`for _, sub := range r.substrings` with no fallback), and does not know HTTP status. Extend per RESEARCH F6: add `status int` (0 = any) and allow empty `substrings` to match on kind/status/op alone:
```go
type writeRule struct {
	kind glclient.Kind
	op   string
	// NEW: status int  // 0 = any
	substrings []string
	text       string
}
// match: r.kind == e.Kind && (r.op == "" || r.op == op) && (r.status == 0 || r.status == e.Status)
//        && (len(r.substrings) == 0 || any lowercase substring in lower(detail))
```
Existing rules must keep their behaviour (all have non-empty substrings, so the change is backward compatible; the `errors_test.go` write cases at lines 55-128 must still pass unchanged).

**Rule placement:** MR rules keyed on their op go in `writeRules` BEFORE the generic ones (rules run first, so a merge 401 rule wins over the kind-level `KindUnauthorized` branch at line 219, which says "токен недействителен ... scope api" and is wrong for merge). Texts carry no status digits (`statusPrefix(e)` is prepended at line 210). Rules from RESEARCH lines 354-363 (409 duplicate, same-branch, does-not-exist, 401 merge = no permission, 405/406 merge, 422 "branch cannot be merged", 400 "sha must be provided", 403 merge). KindForbidden on merge needs a merge-specific rule (no permission to merge into the protected target branch) placed before the switch at line 215.

**999.2 (writeUnknownOutcome)** (lines 189-192, 221-222): today it is applied only for `KindServer, KindTimeout, KindNetwork` and the suffix names "list_branches, list_commits":
```go
const writeUnknownOutcome = " Результат записи неизвестен: изменение могло быть применено. " +
	"Проверьте состояние (list_branches, list_commits) перед повтором."
...
case glclient.KindServer, glclient.KindTimeout, glclient.KindNetwork:
	return baseText(e, subject) + writeUnknownOutcome, true
```
Change to: extend the case to `KindCanceled, KindDecode, KindTooLarge, KindOther`, and select the hint by `op` (`opCreateMR` -> list_merge_requests source_branch=..., `opMergeMR`/`opUpdateMR` -> get_merge_request, `opMRNote` -> list_merge_request_notes; existing ops keep the branch/commit hint). Keep the literal phrase `Результат записи неизвестен` (tests at `branch_test.go` line 256 and `errors_test.go` lines 116-122 assert it). `KindRateLimited` (429) stays the plain rate-limit text (rejected before processing). Watch: a `KindOther` write error from a handler guard is returned WITHOUT `withWrite`, so it is unaffected; only errors wrapped by `withWrite` reach `writeText`. Also review `baseText` `KindServer` ("повторите позже", line 97), which contradicts the unknown-outcome suffix (02-REVIEW WR-01).

**Tests:** extend the table in `errors_test.go` (fields `prefix`, `contains`, `notContains`, `exact`; entries built as `withWrite(opMergeMR, "MR (iid) или проект", &glclient.Error{Kind: glclient.KindBadRequest, Status: 405})`), including Canceled/Decode/TooLarge/Other cases for 999.2 and a case proving the merge 401 text does NOT say "токен недействителен".

---

### `internal/tools/register.go` (modify)

**Analog:** itself, lines 40-81. Read tools:
```go
mcp.AddTool(s, &mcp.Tool{
	Name:        "list_commits",
	Description: listCommitsDescription,
	Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
}, safe(d, listCommits(d)))
```
Write tools (lines 64-68, 77-81):
```go
Annotations: &mcp.ToolAnnotations{DestructiveHint: gitlab.Ptr(false)},   // additive: create_*
Annotations: &mcp.ToolAnnotations{DestructiveHint: gitlab.Ptr(true)},    // update/merge
```
Assign: `list_merge_requests`, `get_merge_request`, `get_merge_request_diffs`, `list_merge_request_notes` -> `ReadOnlyHint: true`; `create_merge_request`, `create_merge_request_note` -> `DestructiveHint: gitlab.Ptr(false)`; `update_merge_request`, `merge_merge_request` -> `DestructiveHint: gitlab.Ptr(true)`. `gitlab` is already imported in this file. No explicit `InputSchema` needed: all 8 inputs are flat (unlike `commit_files`, which needed `schema.go`).

---

### Tests: `mr_test.go`, `mr_diff_test.go`, `mr_notes_test.go`, `mr_merge_test.go`, `mr_status_test.go`

**Analog:** `branch_test.go` (write flows, wire assertions) and `commits_test.go` lines 206-322 (`get_commit`).

**Session + fake + call** (`branch_test.go` lines 109-116; helpers in `helpers_test.go` lines 35-65). Register RAW encoded paths (`g%2Fp`, project id/path in `fake.JSON`):
```go
fake := testutil.NewFakeGitLab(t)
fake.JSON("POST", branchesPath, 201, createdJSON, nil)
cs := newTestSession(t, fake)
text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/x", "ref": "main"})
if isErr {
	t.Fatalf("unexpected tool error: %s", text)
}
```
Define package-level path consts like `branchesPath` (line 16): e.g. `mrPath = "/api/v4/projects/g%2Fp/merge_requests"`. `treeProjectJSON` is already declared in `tree_test.go` line 13; reuse it for the default-branch GET in `create_merge_request` tests. `callText` fails the test if the token leaks (automatic).

**Wire assertions** to copy:
- Exact request list/order: `fake.Requests()` compared to `"GET "+path+"?page=1&per_page=20"` (`branch_test.go` lines 33-36). For list MR assert `state=opened` is on the wire by default via `url.ParseQuery` on the query part (lines 67-74) and that blank filters are not sent (lines 76-82).
- POST/PUT body: `rec := fake.Recorded()`; `json.Unmarshal([]byte(rec[0].Body), &body)`; assert optional merge/update fields ABSENT when not passed (lines 118-128 use `len(body) == 2`).
- Zero-request guards: `if reqs := fake.Requests(); len(reqs) != 0` (lines 162-173) for empty title/body/iid<=0/update-with-nothing/blocked... (blocked merge: exactly one GET, NO `PUT .../merge`).
- Write sent once on 5xx and unknown-outcome text: `TestCreateBranchServerErrorIsSentOnce` (lines 247-262) using `requestsTo(fake, "PUT ...")` (helper at `projects_test.go` line 13); assert the hint mentions `get_merge_request`.
- Real status in prefix: loop over statuses as in `TestCreateBranchAlreadyExistsShowsRealStatus` (lines 264-279).
- Next-page footer, both `X-Next-Page` and Link-only: copy `TestListBranchesNextPageFooter` (lines 85-107) for MR list, diffs and notes.
- Budget/diff: copy `TestGetCommitBudgetedDiff` (`commits_test.go` lines 276-304; helper `patchOf`) for `get_merge_request_diffs`; assert `changes_count:"1000+"` yields the overflow line and an empty patch with `too_large` never reads as "no changes".
- Stateful fake for the end-to-end MR flow (create -> get -> merge): mirror `TestCreateThenListShowsNewBranch` (lines 189-223) with `fake.Handle`.

**Route JSON to register** (from RESEARCH lines 376-380): `GET .../merge_requests` (list), `.../merge_requests/5` (single, with `detailed_merge_status`, `changes_count` as a STRING), `.../merge_requests/5/diffs` (no `overflow` key), `PUT .../merge_requests/5/merge`, `POST .../merge_requests` (409 body `{"message":["Another open merge request already exists for this source branch: !5"]}`), `GET .../repository/compare` (for the create pre-check, `commits` empty vs non-empty). Unregistered routes answer 404, which hides missing routes: always assert `fake.Requests()`.

---

### `internal/tools/schema_test.go` / `testdata/tools_list.golden.json`

**Analog:** `TestToolSchemas` and `TestToolsListGolden` (already present). No code change: they iterate every registered tool (name <=30, description 1..900 runes, no `$ref/anyOf/oneOf`, no `null` in `type`, every property has a description). Regenerate the golden: `go test ./internal/tools -run TestToolsListGolden -update`. Consequences for input structs: every field needs a `jsonschema` tag; `iid` is `int` (not `*int`); `draft`, `squash`, `remove_source_branch` are plain `bool` with `omitempty`.

---

### `scripts/smoke.py`, `e2e/python_smoke_test.go`, `e2e/stdio_test.go` (modify)

**Analogs:** themselves.
- `scripts/smoke.py` lines 36-49: extend `EXPECTED_TOOLS` with the 8 new names (20 total). New hermetic calls via the existing `call(name, arguments, expect_error=False)` helper (line 143), inside `if base_url:` (line 167). New WRITE calls (create/note/update/merge) must run only against the fake: 999.5 is still open, `--base-url` is currently treated as hermetic; either add a loopback-only check for the write block or leave writes guarded by `if base_url:` plus an explicit `127.0.0.1`/`localhost` assertion (RESEARCH P14).
- `e2e/python_smoke_test.go` lines 23-77: add `fake.JSON(...)` routes for the new endpoints (same raw-path style; `commitRoute` variable pattern for repeated prefixes).
- `e2e/stdio_test.go` line 197: `wantNames` is a HARD-CODED sorted list of the 12 tools; it fails as soon as `Register` gets 8 more tools. Add the 8 names in sorted position (RESEARCH did not list this file; it is a required edit). Check `e2e/purity_test.go` is unaffected (it asserts zero network requests on `tools/list`, which stays true).

---

## Shared Patterns

### Handler shape and input normalisation
**Source:** `branch.go` lines 115-136, `commits.go` lines 80-104
**Apply to:** all 8 handlers
`func name(d Deps) func(ctx context.Context, in XxxIn) (string, error)`, `glclient.NormalizeProject(in.Project)` first (Russian error returned unwrapped), then trim/guard string arguments, reject empty required values with a plain `errors.New("не указано ...")` BEFORE any request, `ClampPaging` for lists, one client-go call with `gitlab.WithContext(ctx)`. Handlers never build `isError` results themselves; returning an error is what makes `isError: true` (via `safe`).

### Error handling
**Source:** `errors.go` lines 34-39, 50-55, 59-78; `safe.go` (`safe(d, handler)` recovers panics, 25 s timeout, converts errors via `toToolText`, redacts token last)
**Apply to:** every client-go call
```go
if err != nil {
	return "", withSubject("MR (iid) или проект", err)        // reads and pre-reads before a write
}
...
return "", withWrite(opMergeMR, "MR (iid) или проект", err)   // the state-changing request itself
```
Never wrap a read (or a pre-check GET) in `withWrite`; never wrap a local guard error at all. Never echo raw bodies (`capDetail`, 300 runes). Texts Russian.

### Output budget and pagination
**Source:** `format.go` lines 19-107
**Apply to:** list_merge_requests, list_merge_request_notes, get_merge_request_diffs, all bounded single-object outputs
`OutputBudget = 15000` runes; `Budget(body, limit)` (rune-based, cuts on a line boundary); `ClampPaging` (default 20, max 100); `PageFooter`; `composeList`; `oneLine(s, max)`. Order: body, `TruncatedFooter` if cut, then page footer. Never read `X-Total`. Named `const` caps (`mrDescriptionRunes`, `noteBodyRunes`, `noteSystemLineRunes`) in the style of `commitTitleRunes`/`branchTitleRunes`/`commitMessageRunes`.

### Default ref resolution
**Source:** `ref.go` lines 19-39
**Apply to:** `create_merge_request` only (empty `target_branch`); `refLabel(target, isDefault)` in the output. Explicit ref costs no request; empty costs one `GetProject` and yields `errEmptyRepo` for empty repos.

### Write safety (no retry)
**Source:** `internal/glclient/retry.go` (GET/HEAD-only retry policy, already covers POST/PUT); `branch_test.go` lines 247-262
**Apply to:** create_merge_request, update_merge_request, merge_merge_request, create_merge_request_note
No retry loops, no "verify then retry" inside a handler. The only extra requests allowed around a write are reads BEFORE it (default-branch GET, compare, MR GET, list-by-source_branch on a 409). A 5xx/timeout/network (and, after 999.2, canceled/decode/toolarge/other) failure must say the outcome is unknown.

### Tool schema constraints (FND-08)
**Source:** `schema_test.go`; `branch.go` lines 33-45
**Apply to:** all input structs: plain fields, `json:"x,omitempty"` for optional, English `jsonschema:"..."` on every field, no pointers, description <=900 runes, name <=30 chars (`list_merge_request_notes` = 24, `create_merge_request_note` = 25, `get_merge_request_diffs` = 23).

### Test harness
**Source:** `helpers_test.go` lines 22-65; `internal/testutil/fakegitlab.go` (`JSON`, `Handle`, `Requests`, `Recorded`, `Reset`)
**Apply to:** all new tests. Mock at the HTTP boundary (fake server), not at the client interface; use the in-memory MCP session so go-sdk schema validation is exercised.

## No Analog Found

| File / Element | Role | Data Flow | Reason |
|----------------|------|-----------|--------|
| Draft title-prefix helper (`withDraftPrefix`, regex) | utility | transform | Nothing similar in the repo; use RESEARCH Pattern 3 (lines 184-191). |
| `!N` extraction from a 409 detail + fallback list lookup | utility | request-response | No existing recovery-lookup after a failed write; use RESEARCH lines 366-372 and Pitfall 5. Keep it read-only and never convert to success. |
| `writeRule.status` matching (kind + status + op, empty `substrings`) | utility | transform | Extends the existing matcher; no prior rule matches on status alone (405/422 merge). |
| Overflow surrogate from `changes_count` (`"1000+"`) | utility | transform | `/diffs` has no `overflow`; RESEARCH F1. |

## Conventions

Convention derivation is unavailable for this Go tree (the shared deriver returned `no-readable-files` in Phase 2; not re-run). Hand-observed in `internal/**`:

| Axis | Dominant | Share | Entropy | Status |
|------|----------|-------|---------|--------|
| File-name casing | lowercase single word `.go`; tests `<name>_test.go` (`branch.go`, `commits.go`, `branch_test.go`); no underscores in production names except tests | ~100% of files in `internal/tools`, `internal/testutil` | low | named contract |
| Identifier casing | exported `PascalCase` (`BranchesIn`, `OutputBudget`, `ClampPaging`), unexported `camelCase` (`branchLine`, `resolveRef`, `commitTitleRunes`) | ~100% | low | named contract |
| Export style | package-level; tool handlers are unexported constructors `func name(d Deps) func(ctx, in T) (string, error)`; inputs are exported `XxxIn` structs; description consts `<tool>Description` | ~100% | low | named contract |
| Import style | three groups separated by blank lines: stdlib, third-party (`gitlab "gitlab.com/gitlab-org/api/client-go/v2"`, `mcp`), module-local `gitlab-mcp/internal/...` | ~100% | low | named contract |

Note for the planner: RESEARCH proposes multi-part names (`mr_status.go`, `mr_diff.go`, `mr_notes.go`, `mr_merge.go`). Existing production files are single words; the underscore form is still lowercase/Go-valid (tests already use `_test`), so this is acceptable but not previously used in production files. Alternative: fold status into `mr.go`. Author's choice.

**Contested hotspots (author's choice):** none within this Go module. The CJS<->SDK dual resolver split (`bin/lib/**` CJS `module.exports`/`require` vs `sdk/src/**` ESM `export`/`import`) is a property of the GSD plugin repository, not this project: there is no such split here, so match the directory's local style (Go, `internal/tools`). Additional local conventions: user-facing strings and tool descriptions in Russian, `jsonschema` tag text in English, comments in English, one concern per file.

## Metadata

**Analog search scope:** `internal/tools`, `internal/glclient`, `internal/testutil`, `scripts`, `e2e`
**Files read:** `errors.go`, `commits.go`, `diff.go`, `compare.go`, `branch.go`, `write.go` (lines 1-340), `register.go`, `format.go`, `ref.go`, `helpers_test.go`, `branch_test.go`, `errors_test.go`, `commits_test.go` (lines 205-323), `testutil/fakegitlab.go`, `glclient/errors.go` (Classify), `scripts/smoke.py` (lines 30-80), `e2e/stdio_test.go` (lines 160-220); client-go v2.64.0 method signatures verified via grep in the module cache
**Not re-read (per 02-PATTERNS/RESEARCH):** `safe.go`, `glclient/{client,retry,transport}.go`, `testutil/session.go`, `e2e/purity_test.go`
**Pattern extraction date:** 2026-09-24
