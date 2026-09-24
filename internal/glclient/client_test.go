package glclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/config"
	"gitlab-mcp/internal/testutil"
)

const testToken = "glpat-TESTSECRET"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testClient(t *testing.T, baseURL string, l limits) *gitlab.Client {
	t.Helper()
	c, err := newClient(config.Config{Token: testToken, BaseURL: baseURL}, discardLogger(), l)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c
}

// do sends a raw request through the client so that the retry policy is
// exercised without depending on any typed service.
func do(t *testing.T, c *gitlab.Client, method string, opts ...gitlab.RequestOptionFunc) error {
	t.Helper()
	req, err := c.NewRequest(method, "test", nil, opts)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = c.Do(req, nil)
	return err
}

func TestRetryPolicyAttempts(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		status   int
		headers  map[string]string
		attempts int
		minTime  time.Duration
		maxTime  time.Duration
	}{
		{name: "GET 503 retried twice", method: "GET", status: 503, attempts: 3, maxTime: 3 * time.Second},
		{name: "GET 429 short Retry-After retried twice", method: "GET", status: 429,
			headers: map[string]string{"Retry-After": "1"}, attempts: 3,
			minTime: 1900 * time.Millisecond, maxTime: 4 * time.Second},
		{name: "GET 429 long Retry-After surfaced", method: "GET", status: 429,
			headers: map[string]string{"Retry-After": "30"}, attempts: 1, maxTime: time.Second},
		{name: "GET 404 not retried", method: "GET", status: 404, attempts: 1, maxTime: time.Second},
		{name: "GET 501 not retried", method: "GET", status: 501, attempts: 1, maxTime: time.Second},
		{name: "POST 500 never retried", method: "POST", status: 500, attempts: 1, maxTime: time.Second},
		{name: "POST 429 never retried", method: "POST", status: 429,
			headers: map[string]string{"Retry-After": "1"}, attempts: 1, maxTime: time.Second},
		{name: "PUT 503 never retried", method: "PUT", status: 503, attempts: 1, maxTime: time.Second},
		{name: "DELETE 503 never retried", method: "DELETE", status: 503, attempts: 1, maxTime: time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := testutil.NewFakeGitLab(t)
			f.JSON(tc.method, "/api/v4/test", tc.status, `{"message":"boom"}`, tc.headers)
			c := testClient(t, f.URL, limits{maxBody: defaultMaxBody})

			start := time.Now()
			err := do(t, c, tc.method)
			elapsed := time.Since(start)

			if err == nil {
				t.Fatalf("expected an error for status %d", tc.status)
			}
			if got := len(f.Requests()); got != tc.attempts {
				t.Errorf("attempts = %d, want %d (requests: %v)", got, tc.attempts, f.Requests())
			}
			if elapsed < tc.minTime || elapsed > tc.maxTime {
				t.Errorf("elapsed = %v, want within [%v, %v]", elapsed, tc.minTime, tc.maxTime)
			}
		})
	}
}

func TestBackoff(t *testing.T) {
	resp := func(status int, retryAfter string) *http.Response {
		h := http.Header{}
		if retryAfter != "" {
			h.Set("Retry-After", retryAfter)
		}
		return &http.Response{StatusCode: status, Header: h}
	}
	tests := []struct {
		name string
		resp *http.Response
		want time.Duration
	}{
		{"429 with Retry-After", resp(429, "3"), 3 * time.Second},
		{"429 without header", resp(429, ""), time.Second},
		{"429 capped at 5 s", resp(429, "99"), 5 * time.Second},
		{"503 fixed", resp(503, ""), 500 * time.Millisecond},
		{"nil response", nil, 500 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := backoff(0, 0, 0, tc.resp); got != tc.want {
				t.Errorf("backoff = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLimiterIgnoresRateLimitHeaders(t *testing.T) {
	f := testutil.NewFakeGitLab(t)
	reset := strconv.FormatInt(time.Now().Add(50*time.Second).Unix(), 10)
	f.JSON("GET", "/api/v4/test", 200, `{}`, map[string]string{
		"RateLimit-Reset": reset,
		"RateLimit-Limit": "1",
	})
	c := testClient(t, f.URL, limits{maxBody: defaultMaxBody})

	start := time.Now()
	for i := 0; i < 2; i++ {
		if err := do(t, c, "GET"); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("two GETs took %v, want < 1s (limiter must not sleep)", elapsed)
	}
}

func TestDeadlineEndsSlowCall(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c := testClient(t, srv.URL, limits{maxBody: defaultMaxBody})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := do(t, c, "GET", gitlab.WithContext(ctx))
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > time.Second {
		t.Errorf("elapsed = %v, want < 1s", elapsed)
	}
}

func TestBodyCap(t *testing.T) {
	f := testutil.NewFakeGitLab(t)
	big := `{"data":"` + strings.Repeat("a", 3<<20) + `"}`
	f.JSON("GET", "/api/v4/test", 200, big, nil)
	c := testClient(t, f.URL, limits{maxBody: 1 << 20})

	var out map[string]any
	req, err := c.NewRequest("GET", "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Do(req, &out)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("err = %v, want ErrBodyTooLarge", err)
	}
}

func TestRefusedConnection(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := testClient(t, url, limits{maxBody: defaultMaxBody})

	start := time.Now()
	err := do(t, c, "GET")
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("elapsed = %v, want < 5s", elapsed)
	}
}

func TestRefusedConnectionWriteNotRetried(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := testClient(t, url, limits{maxBody: defaultMaxBody})

	start := time.Now()
	if err := do(t, c, "POST"); err == nil {
		t.Fatal("expected a connection error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("POST to a refused connection took %v, want no retry waiting", elapsed)
	}
}
