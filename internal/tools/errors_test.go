package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

func TestToToolText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		// contains / prefix are both optional; notContains is optional.
		prefix      string
		contains    []string
		notContains []string
		exact       string
	}{
		{name: "401", err: &glclient.Error{Kind: glclient.KindUnauthorized, Status: 401},
			prefix: "401: GitLab отклонил токен"},
		{name: "403", err: &glclient.Error{Kind: glclient.KindForbidden, Status: 403},
			prefix: "403: недостаточно прав"},
		{name: "404 with subject", err: withSubject("файл", &glclient.Error{Kind: glclient.KindNotFound, Status: 404}),
			contains: []string{"404", "файл"}},
		{name: "404 without subject", err: &glclient.Error{Kind: glclient.KindNotFound, Status: 404},
			contains: []string{"проект, путь или ref"}},
		{name: "429 with retry", err: &glclient.Error{Kind: glclient.KindRateLimited, Status: 429, RetryAfter: 42 * time.Second},
			contains: []string{"повторите через 42 с"}},
		{name: "429 default", err: &glclient.Error{Kind: glclient.KindRateLimited, Status: 429},
			contains: []string{"повторите через 60 с"}},
		{name: "502", err: &glclient.Error{Kind: glclient.KindServer, Status: 502, Detail: "<html>bad gateway</html>"},
			contains:    []string{"502: ошибка сервера GitLab, повторите позже"},
			notContains: []string{"<html"}},
		{name: "400", err: &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "bad thing"},
			prefix: "400: GitLab отклонил запрос: bad thing"},
		{name: "422", err: &glclient.Error{Kind: glclient.KindBadRequest, Status: 422, Detail: "nope"},
			prefix: "422: GitLab отклонил запрос: nope"},
		{name: "timeout", err: &glclient.Error{Kind: glclient.KindTimeout},
			contains: []string{"Превышено время ожидания (25 с)"}},
		{name: "network", err: &glclient.Error{Kind: glclient.KindNetwork, Detail: "connection refused"},
			prefix: "Не удалось связаться с GitLab"},
		{name: "canceled", err: &glclient.Error{Kind: glclient.KindCanceled}, contains: []string{"отменён"}},
		{name: "too large", err: &glclient.Error{Kind: glclient.KindTooLarge}, contains: []string{"слишком большой"}},
		{name: "decode", err: &glclient.Error{Kind: glclient.KindDecode}, contains: []string{"Неожиданный ответ GitLab"}},
		{name: "other passes detail", err: errors.New("не указан проект"), exact: "не указан проект"},

		{name: "write 400 branch exists",
			err:    withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: Branch already exists}"}),
			prefix: "400: ветка уже существует"},
		{name: "write 409 branch exists",
			err:         withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 409, Detail: "{message: Branch already exists}"}),
			prefix:      "409: ветка уже существует",
			notContains: []string{"400:"}},
		{name: "write 400 invalid ref",
			err:      withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: Failed to create branch 'x': invalid reference name 'nope'}"}),
			prefix:   "400: исходный ref не найден",
			contains: []string{"invalid reference name 'nope'"}},
		{name: "write 422 invalid ref",
			err:    withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 422, Detail: "{message: Failed to create branch 'x': invalid reference name 'nope'}"}),
			prefix: "422: исходный ref не найден"},
		{name: "write invalid branch name",
			err:    withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: Branch name is invalid}"}),
			prefix: "400: недопустимое имя ветки"},
		{name: "branch-exists rule is keyed on the op",
			err:         withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: Branch already exists}"}),
			notContains: []string{"ветка уже существует"},
			exact:       "400: GitLab отклонил запрос: {message: Branch already exists}"},
		{name: "commit protected branch",
			err:   withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: You are not allowed to push into this branch}"}),
			exact: "400: ветка защищена: создайте ветку (create_branch), коммитьте туда, затем MR."},
		{name: "commit protected branch 403",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindForbidden, Status: 403, Detail: "{message: You are not allowed to push into this branch}"}),
			prefix:   "403: запись отклонена",
			contains: []string{"ветка может быть защищена", "scope `api`", "Developer"}},
		{name: "commit missing branch",
			err:   withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: You can only create or edit files when you are on a branch}"}),
			exact: "400: ветка не найдена: создайте её через create_branch."},
		{name: "commit file exists 400",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: A file with this name already exists}"}),
			prefix:   "400: файл уже существует",
			contains: []string{"action=update"}},
		{name: "commit file exists 422",
			err:         withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 422, Detail: "{message: A file with this name already exists}"}),
			prefix:      "422: файл уже существует",
			notContains: []string{"ветка уже существует"}},
		{name: "commit file missing (doesn't exist)",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: A file with this name doesn't exist}"}),
			contains: []string{"файла нет на ветке", "action=create"}},
		{name: "commit file missing (does not exist)",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: A file with this name does not exist}"}),
			contains: []string{"файла нет на ветке", "action=create"}},
		{name: "commit stale content 400",
			err:   withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: The file has changed since you started editing it: docs/new.md}"}),
			exact: "400: файл изменился с момента чтения — прочитайте его заново (get_file_contents) и повторите."},
		{name: "commit stale content 409",
			err:         withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 409, Detail: "{message: The file has changed since you started editing it: docs/new.md}"}),
			prefix:      "409: файл изменился с момента чтения",
			notContains: []string{"400:"}},
		{name: "write 403",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindForbidden, Status: 403}),
			prefix:   "403: запись отклонена",
			contains: []string{"ветка может быть защищена", "scope `api`", "Developer", "create_branch"}},
		{name: "write 401",
			err:      withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindUnauthorized, Status: 401}),
			contains: []string{"401", "scope `api`"}},
		{name: "write 503 outcome unknown",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindServer, Status: 503}),
			contains: []string{"503: ошибка сервера GitLab", "Результат записи неизвестен"}},
		{name: "write timeout outcome unknown",
			err:      withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindTimeout}),
			contains: []string{"Превышено время ожидания (25 с)", "Результат записи неизвестен"}},
		{name: "write network outcome unknown",
			err:      withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindNetwork, Detail: "connection reset"}),
			contains: []string{"Не удалось связаться с GitLab", "Результат записи неизвестен"}},
		{name: "write 400 unmatched detail is generic",
			err:   withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "bad thing"}),
			exact: "400: GitLab отклонил запрос: bad thing"},
		{name: "write 404 uses the subject",
			err:      withWrite(opCreateBranch, "проект или ветка", &glclient.Error{Kind: glclient.KindNotFound, Status: 404}),
			contains: []string{"404: не найдено (проект или ветка)"}},
		{name: "read 503 has no outcome warning",
			err:         withSubject("проект", &glclient.Error{Kind: glclient.KindServer, Status: 503}),
			notContains: []string{"Результат записи неизвестен"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toToolText(tc.err)
			if tc.prefix != "" && !strings.HasPrefix(got, tc.prefix) {
				t.Errorf("text %q does not start with %q", got, tc.prefix)
			}
			for _, s := range tc.contains {
				if !strings.Contains(got, s) {
					t.Errorf("text %q does not contain %q", got, s)
				}
			}
			for _, s := range tc.notContains {
				if strings.Contains(got, s) {
					t.Errorf("text %q must not contain %q", got, s)
				}
			}
			if tc.exact != "" && got != tc.exact {
				t.Errorf("text = %q, want %q", got, tc.exact)
			}
		})
	}
}

func TestToToolTextCapsBadRequestDetail(t *testing.T) {
	long := strings.Repeat("ж", 1000)
	got := toToolText(&glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: long})
	if n := len([]rune(got)); n > 400 {
		t.Errorf("text has %d runes, want the detail capped near 300", n)
	}
}

func TestToToolTextClassifiesRawErrors(t *testing.T) {
	if got := toToolText(context.DeadlineExceeded); !strings.Contains(got, "Превышено время ожидания (25 с)") {
		t.Errorf("deadline text = %q", got)
	}
	if got := toToolText(gitlab.ErrNotFound); !strings.Contains(got, "404") {
		t.Errorf("not found text = %q", got)
	}
	if got := toToolText(fmt.Errorf("wrap: %w", withSubject("проект", gitlab.ErrNotFound))); !strings.Contains(got, "проект") {
		t.Errorf("wrapped subject lost: %q", got)
	}
}

func TestSafeDeadlineBecomesReadableError(t *testing.T) {
	d := Deps{Timeout: 200 * time.Millisecond}
	h := safe(d, func(ctx context.Context, _ struct{}) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	start := time.Now()
	res, _, err := h(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want < 1s", elapsed)
	}
	if !res.IsError {
		t.Fatal("IsError = false, want true")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	if !strings.Contains(tc.Text, "Превышено время ожидания (25 с)") {
		t.Errorf("text = %q", tc.Text)
	}
}
