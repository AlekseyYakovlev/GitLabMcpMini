package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

func TestResolveRefExplicitMakesNoRequest(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	d := newTestDeps(t, fake)

	ref, isDefault, err := resolveRef(context.Background(), d, "g/p", "feature/x")
	if err != nil {
		t.Fatalf("resolveRef: %v", err)
	}
	if ref != "feature/x" || isDefault {
		t.Errorf("got (%q, %v), want (feature/x, false)", ref, isDefault)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("requests = %v, want none", reqs)
	}
}

func TestResolveRefDefaultBranch(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"develop"}`, nil)
	d := newTestDeps(t, fake)

	ref, isDefault, err := resolveRef(context.Background(), d, "g/p", "")
	if err != nil {
		t.Fatalf("resolveRef: %v", err)
	}
	if ref != "develop" || !isDefault {
		t.Errorf("got (%q, %v), want (develop, true)", ref, isDefault)
	}
	if ref == "HEAD" {
		t.Errorf("ref must never be HEAD")
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET /api/v4/projects/g%2Fp" {
		t.Errorf("requests = %v, want exactly GET /api/v4/projects/g%%2Fp", reqs)
	}
}

func TestResolveRefEmptyRepo(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"","empty_repo":true}`, nil)
	d := newTestDeps(t, fake)

	_, _, err := resolveRef(context.Background(), d, "g/p", "")
	if !errors.Is(err, errEmptyRepo) {
		t.Fatalf("err = %v, want errEmptyRepo", err)
	}
}

func TestResolveRefNoDefaultBranchIsEmptyRepo(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, `{"id":1,"path_with_namespace":"g/p","default_branch":""}`, nil)
	d := newTestDeps(t, fake)

	_, _, err := resolveRef(context.Background(), d, "g/p", "")
	if !errors.Is(err, errEmptyRepo) {
		t.Fatalf("err = %v, want errEmptyRepo", err)
	}
}

func TestResolveRefProjectNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	d := newTestDeps(t, fake)

	_, _, err := resolveRef(context.Background(), d, "g/p", "")
	if err == nil {
		t.Fatal("expected an error for a missing project")
	}
	text := toToolText(err)
	if !strings.Contains(text, "404") || !strings.Contains(text, "проект") {
		t.Errorf("toToolText = %q, want 404 and 'проект'", text)
	}
}

func TestRefLabel(t *testing.T) {
	if got := refLabel("main", true); got != "main (ветка по умолчанию)" {
		t.Errorf("refLabel(main, true) = %q", got)
	}
	if got := refLabel("v1.0", false); got != "v1.0" {
		t.Errorf("refLabel(v1.0, false) = %q", got)
	}
}
