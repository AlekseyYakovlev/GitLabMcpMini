package glclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/testutil"
)

// currentUserErr calls a real client-go endpoint against a fake GitLab that
// answers status/body/headers, and returns the resulting error.
func currentUserErr(t *testing.T, status int, body string, headers map[string]string) error {
	t.Helper()
	f := testutil.NewFakeGitLab(t)
	f.JSON("GET", "/api/v4/user", status, body, headers)
	c := testClient(t, f.URL, limits{maxBody: defaultMaxBody})
	_, _, err := c.Users.CurrentUser()
	if err == nil {
		t.Fatalf("status %d: expected an error", status)
	}
	return err
}

func TestClassifyHTTPStatuses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		headers    map[string]string
		wantKind   Kind
		wantStatus int
		wantRetry  time.Duration
		wantDetail string
	}{
		{name: "401", status: 401, body: `{"message":"401 Unauthorized"}`, wantKind: KindUnauthorized, wantStatus: 401},
		{name: "403", status: 403, body: `{"error":"insufficient_scope"}`, wantKind: KindForbidden, wantStatus: 403},
		{name: "404", status: 404, body: `{"message":"404 Not Found"}`, wantKind: KindNotFound, wantStatus: 404},
		{name: "429 with Retry-After", status: 429, body: `Retry later`,
			headers: map[string]string{"Retry-After": "42"}, wantKind: KindRateLimited, wantStatus: 429, wantRetry: 42 * time.Second},
		{name: "502 HTML body", status: 502, body: `<html>bad gateway</html>`, wantKind: KindServer, wantStatus: 502},
		{name: "400 nested message", status: 400, body: `{"message":{"base":["a","b"]}}`,
			wantKind: KindBadRequest, wantStatus: 400, wantDetail: "a"},
		{name: "405", status: 405, body: `{"message":"405 Method Not Allowed"}`, wantKind: KindBadRequest, wantStatus: 405},
		{name: "409", status: 409, body: `{"message":"conflict"}`, wantKind: KindBadRequest, wantStatus: 409},
		{name: "422", status: 422, body: `{"message":"unprocessable"}`, wantKind: KindBadRequest, wantStatus: 422},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := currentUserErr(t, tc.status, tc.body, tc.headers)
			got := Classify(err)
			if got == nil {
				t.Fatal("Classify returned nil")
			}
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v (err: %v)", got.Kind, tc.wantKind, err)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("Status = %d, want %d", got.Status, tc.wantStatus)
			}
			if got.RetryAfter != tc.wantRetry {
				t.Errorf("RetryAfter = %v, want %v", got.RetryAfter, tc.wantRetry)
			}
			if tc.wantDetail != "" && !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, tc.wantDetail)
			}
			if tc.wantKind == KindServer && got.Detail != "" {
				t.Errorf("KindServer Detail = %q, want empty (bodies are never echoed)", got.Detail)
			}
		})
	}
}

func TestClassifyNotFoundKeepsSentinel(t *testing.T) {
	err := currentUserErr(t, 404, `{"message":"404 Not Found"}`, nil)
	if !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("errors.Is(err, ErrNotFound) = false for %v", err)
	}
	if Classify(err).Kind != KindNotFound {
		t.Error("expected KindNotFound")
	}
	if gitlab.ErrNotFound.Message != "Not Found" {
		t.Errorf("sentinel was mutated: %q", gitlab.ErrNotFound.Message)
	}
}

func TestClassifyNetworkRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := testClient(t, url, limits{maxBody: defaultMaxBody})
	_, _, err := c.Users.CurrentUser()
	if err == nil {
		t.Fatal("expected an error")
	}
	got := Classify(err)
	if got.Kind != KindNetwork {
		t.Errorf("Kind = %v, want KindNetwork (err: %v)", got.Kind, err)
	}
	if got.Detail == "" {
		t.Error("network Detail must carry the cause")
	}
}

func TestClassifyBodyTooLarge(t *testing.T) {
	f := testutil.NewFakeGitLab(t)
	f.JSON("GET", "/api/v4/user", 200, `{"name":"`+strings.Repeat("a", 3<<20)+`"}`, nil)
	c := testClient(t, f.URL, limits{maxBody: 1 << 20})
	_, _, err := c.Users.CurrentUser()
	if got := Classify(err); got == nil || got.Kind != KindTooLarge {
		t.Errorf("Classify = %+v, want KindTooLarge (err: %v)", got, err)
	}
}

func TestClassifyDirect(t *testing.T) {
	var syntaxErr *json.SyntaxError
	if e := json.Unmarshal([]byte("{bad"), &map[string]any{}); !errors.As(e, &syntaxErr) {
		t.Fatalf("setup: expected a syntax error, got %v", e)
	}
	tests := []struct {
		name       string
		err        error
		wantKind   Kind
		wantDetail string
	}{
		{"deadline", context.DeadlineExceeded, KindTimeout, ""},
		{"wrapped deadline", fmt.Errorf("Get x: %w", context.DeadlineExceeded), KindTimeout, ""},
		{"canceled", context.Canceled, KindCanceled, ""},
		{"too large wrapped", fmt.Errorf("read: %w", ErrBodyTooLarge), KindTooLarge, ""},
		{"json syntax", syntaxErr, KindDecode, ""},
		{"other", errors.New("x"), KindOther, "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if got == nil || got.Kind != tc.wantKind {
				t.Fatalf("Classify = %+v, want kind %v", got, tc.wantKind)
			}
			if got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
		})
	}
	if Classify(nil) != nil {
		t.Error("Classify(nil) must be nil")
	}
}

func TestClassifyDetailCapped(t *testing.T) {
	long := strings.Repeat("я", 1000)
	got := Classify(errors.New(long))
	if n := len([]rune(got.Detail)); n > 300 {
		t.Errorf("Detail has %d runes, want <= 300", n)
	}
}

func TestErrorImplementsError(t *testing.T) {
	var err error = &Error{Kind: KindServer, Status: 502}
	if err.Error() == "" {
		t.Error("Error() must not be empty")
	}
}
