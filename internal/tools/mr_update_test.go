package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

// updatedMRJSON is the PUT answer: an open, non-draft MR.
func updatedMRJSON(extra string) string {
	return mrJSON("mergeable", extra)
}

func newUpdateFake(t testing.TB) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("PUT", mr5Path, 200, updatedMRJSON(""), nil)
	return fake
}

// putBody returns the decoded JSON body of the only PUT the fake saw.
func putBody(t testing.TB, fake *testutil.FakeGitLab) map[string]any {
	t.Helper()
	var bodies []string
	for _, r := range fake.Recorded() {
		if r.Method == "PUT" {
			bodies = append(bodies, r.Body)
		}
	}
	if len(bodies) != 1 {
		t.Fatalf("PUT count = %d, want exactly 1", len(bodies))
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &body); err != nil {
		t.Fatalf("body %q: %v", bodies[0], err)
	}
	return body
}

func countPuts(fake *testutil.FakeGitLab) int {
	return len(requestsTo(fake, "PUT "))
}

func TestUpdateMergeRequestTitleOnly(t *testing.T) {
	fake := newUpdateFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "title": "New"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if reqs := fake.Requests(); len(reqs) != 1 || reqs[0] != "PUT "+mr5Path {
		t.Fatalf("requests = %v, want exactly one PUT", reqs)
	}
	if body := putBody(t, fake); len(body) != 1 || body["title"] != "New" {
		t.Errorf("PUT body = %v, want exactly {title: New}", body)
	}
	for _, want := range []string{
		"MR !5 обновлён: title",
		"state: opened",
		"draft: нет",
		"ветки: f→main",
		"https://gitlab.example/g/p/-/merge_requests/5",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
}

func TestUpdateMergeRequestSeveralFields(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("PUT", mr5Path, 200, strings.Replace(updatedMRJSON(""), `"state":"opened"`, `"state":"closed"`, 1), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{
		"project": "g/p", "iid": 5, "description": "d", "target_branch": "dev", "state_event": "close",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	body := putBody(t, fake)
	if len(body) != 3 || body["description"] != "d" || body["target_branch"] != "dev" || body["state_event"] != "close" {
		t.Errorf("PUT body = %v, want exactly description, target_branch, state_event", body)
	}
	for _, want := range []string{"description, target_branch, state_event", "state: closed"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
}

func TestUpdateMergeRequestReopen(t *testing.T) {
	fake := newUpdateFake(t)
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "state_event": "reopen"}); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if body := putBody(t, fake); len(body) != 1 || body["state_event"] != "reopen" {
		t.Errorf("PUT body = %v", body)
	}
}

func TestUpdateMergeRequestGuardsSendNothing(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"merge is not a state event", map[string]any{"project": "g/p", "iid": 5, "state_event": "merge"}, "close или reopen"},
		{"open is not a state event", map[string]any{"project": "g/p", "iid": 5, "state_event": "open"}, "close или reopen"},
		{"nothing to change", map[string]any{"project": "g/p", "iid": 5},
			"нечего менять: передайте title, description, target_branch, state_event или draft=true"},
		{"blank fields only", map[string]any{"project": "g/p", "iid": 5, "title": "  ", "description": " "}, "нечего менять"},
		{"zero iid", map[string]any{"project": "g/p", "iid": 0, "title": "x"}, "не указан iid MR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpdateFake(t)
			fake.JSON("GET", mr5Path, 200, updatedMRJSON(""), nil)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "update_merge_request", tc.args)
			if !isErr || !strings.Contains(text, tc.want) {
				t.Fatalf("isErr=%v text=%q, want error containing %q", isErr, text, tc.want)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("no request expected, got %v", reqs)
			}
		})
	}
}

func TestUpdateMergeRequestDraftWithTitle(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		{"plain", "New", "Draft: New"},
		{"already prefixed", "Draft: New", "Draft: New"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpdateFake(t)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "update_merge_request", map[string]any{
				"project": "g/p", "iid": 5, "title": tc.title, "draft": true,
			})
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			if reqs := fake.Requests(); len(reqs) != 1 {
				t.Fatalf("requests = %v, want one PUT and no GET", reqs)
			}
			if got := putBody(t, fake)["title"]; got != tc.want {
				t.Errorf("title = %v, want %q", got, tc.want)
			}
			if !strings.Contains(text, "title, draft") {
				t.Errorf("text %q must list title and draft as changed", text)
			}
		})
	}
}

func TestUpdateMergeRequestDraftReadsCurrentTitle(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("GET", mr5Path, 200, strings.Replace(updatedMRJSON(""), `"title":"Add x"`, `"title":"Old"`, 1), nil)
	fake.JSON("PUT", mr5Path, 200, strings.Replace(updatedMRJSON(""), `"draft":false`, `"draft":true`, 1), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "draft": true})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	want := []string{"GET " + mr5Path, "PUT " + mr5Path}
	if reqs := fake.Requests(); len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}
	if body := putBody(t, fake); len(body) != 1 || body["title"] != "Draft: Old" {
		t.Errorf("PUT body = %v, want exactly {title: Draft: Old}", body)
	}
	if !strings.Contains(text, "draft: да") {
		t.Errorf("text %q must take draft from the response", text)
	}
}

func TestUpdateMergeRequestAlreadyDraftSendsNoPut(t *testing.T) {
	cases := []struct {
		name, mr string
	}{
		{"draft flag", strings.Replace(updatedMRJSON(""), `"draft":false`, `"draft":true`, 1)},
		{"title prefix", strings.Replace(updatedMRJSON(""), `"title":"Add x"`, `"title":"Draft: Add x"`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpdateFake(t)
			fake.JSON("GET", mr5Path, 200, tc.mr, nil)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "draft": true})
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			if !strings.Contains(text, "MR !5 уже помечен как Draft; изменений нет") {
				t.Errorf("text = %q", text)
			}
			if reqs := fake.Requests(); len(reqs) != 1 || reqs[0] != "GET "+mr5Path {
				t.Errorf("requests = %v, want exactly one GET", reqs)
			}
			if n := countPuts(fake); n != 0 {
				t.Errorf("PUT was sent %d times, want 0", n)
			}
		})
	}
}

func TestUpdateMergeRequestAlreadyDraftStillSendsOtherFields(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("GET", mr5Path, 200, strings.Replace(updatedMRJSON(""), `"draft":false`, `"draft":true`, 1), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "draft": true, "description": "d"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if reqs := fake.Requests(); len(reqs) != 2 || reqs[0] != "GET "+mr5Path || reqs[1] != "PUT "+mr5Path {
		t.Fatalf("requests = %v, want GET then PUT", reqs)
	}
	if body := putBody(t, fake); len(body) != 1 || body["description"] != "d" {
		t.Errorf("PUT body = %v, want exactly {description: d}", body)
	}
}

func TestUpdateMergeRequestUndraftBySendingPlainTitle(t *testing.T) {
	fake := newUpdateFake(t)
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "title": "Ready"}); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if body := putBody(t, fake); len(body) != 1 || body["title"] != "Ready" {
		t.Errorf("PUT body = %v, want exactly {title: Ready}", body)
	}
}

func TestUpdateMergeRequestServerErrorIsSentOnce(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("PUT", mr5Path, 503, `<html>unavailable</html>`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "title": "New"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	for _, want := range []string{"Результат записи неизвестен", "get_merge_request"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
	if n := countPuts(fake); n != 1 {
		t.Errorf("PUT was sent %d times, want exactly 1", n)
	}
}

func TestUpdateMergeRequestNotFound(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("PUT", mr5Path, 404, `{"message":"404 Not found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "title": "New"})
	if !isErr || !strings.Contains(text, "не найдено (MR (iid) или проект)") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
}

func TestUpdateMergeRequestDraftReadNotFoundSendsNoPut(t *testing.T) {
	fake := newUpdateFake(t)
	fake.JSON("GET", mr5Path, 404, `{"message":"404 Not found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "update_merge_request", map[string]any{"project": "g/p", "iid": 5, "draft": true})
	if !isErr || !strings.Contains(text, "не найдено (MR (iid) или проект)") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if n := countPuts(fake); n != 0 {
		t.Errorf("PUT was sent %d times, want 0", n)
	}
}
