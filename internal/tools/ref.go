package tools

import (
	"context"
	"errors"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// errEmptyRepo is returned when a ref is needed but the project has no commits,
// so there is no default branch to fall back to.
var errEmptyRepo = errors.New("репозиторий пуст: в проекте ещё нет коммитов и ветки по умолчанию")

// resolveRef returns the ref a repository tool should use. An explicit ref is
// returned as is without any GitLab request. An empty ref is replaced by the
// project's named default branch (never the symbolic HEAD), which costs one GetProject
// request; isDefault is then true so callers can say so in their output.
// project must already be normalised.
func resolveRef(ctx context.Context, d Deps, project, ref string) (string, bool, error) {
	if ref != "" {
		return ref, false, nil
	}
	p, _, err := d.GL.Projects.GetProject(project, nil, gitlab.WithContext(ctx))
	if err != nil {
		return "", false, withSubject("проект", err)
	}
	if p.EmptyRepo || p.DefaultBranch == "" {
		return "", false, errEmptyRepo
	}
	return p.DefaultBranch, true, nil
}

// refLabel names a ref for a result header, marking the default branch.
func refLabel(ref string, isDefault bool) string {
	if isDefault {
		return ref + " (ветка по умолчанию)"
	}
	return ref
}
