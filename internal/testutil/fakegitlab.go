// Package testutil provides test doubles shared by the unit and end-to-end tests.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// FakeGitLab is an httptest server that imitates the parts of the GitLab REST
// API used by the tests. It records every request it receives.
type FakeGitLab struct {
	// URL is the base URL of the fake server, without a trailing slash.
	URL string

	server *httptest.Server

	mu       sync.Mutex
	routes   map[string]http.HandlerFunc
	requests []string
}

// NewFakeGitLab starts a fake GitLab server that is closed when the test ends.
//
// A single plain handler is used instead of a path-routing multiplexer because
// those clean and redirect paths that contain escaped or unusual segments,
// which would hide the exact bytes the client put on the wire.
func NewFakeGitLab(t testing.TB) *FakeGitLab {
	t.Helper()
	f := &FakeGitLab{routes: make(map[string]http.HandlerFunc)}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	f.URL = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func routeKey(method, rawPath string) string {
	return method + " " + rawPath
}

func (f *FakeGitLab) serve(w http.ResponseWriter, r *http.Request) {
	path := r.RequestURI
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}

	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.RequestURI)
	h := f.routes[routeKey(r.Method, path)]
	f.mu.Unlock()

	if h == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
		return
	}
	h(w, r)
}

// Handle registers h for an exact method and raw request path (query string
// excluded), for example ("GET", "/api/v4/user").
func (f *FakeGitLab) Handle(method, rawPath string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[routeKey(method, rawPath)] = h
}

// JSON registers a route that answers with the given status, JSON body and
// extra response headers.
func (f *FakeGitLab) JSON(method, rawPath string, status int, body string, headers map[string]string) {
	f.Handle(method, rawPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// Requests returns the recorded requests as "METHOD RequestURI" strings, in
// arrival order. The RequestURI is the raw, still percent-encoded form and
// includes the query string.
func (f *FakeGitLab) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.requests))
	copy(out, f.requests)
	return out
}

// Reset forgets all recorded requests. Registered routes are kept.
func (f *FakeGitLab) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}
