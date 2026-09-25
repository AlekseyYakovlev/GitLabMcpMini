package tools

import (
	"context"
	"errors"
	"fmt"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

// developerLevel is the lowest GitLab access level that may write to a
// repository.
const developerLevel = 30

// forbiddenLookupTimeout bounds the extra GET /projects/:id made to explain a
// 403. The lookup is best-effort and must not stall the failing call.
const forbiddenLookupTimeout = 5 * time.Second

// accessLevel returns the token's highest access level on p, taken from the
// project and the group permissions, or 0 when GitLab reported neither.
func accessLevel(p *gitlab.Project) int {
	if p == nil || p.Permissions == nil {
		return 0
	}
	level := 0
	if pa := p.Permissions.ProjectAccess; pa != nil && int(pa.AccessLevel) > level {
		level = int(pa.AccessLevel)
	}
	if ga := p.Permissions.GroupAccess; ga != nil && int(ga.AccessLevel) > level {
		level = int(ga.AccessLevel)
	}
	return level
}

// roleName names a GitLab access level.
func roleName(level int) string {
	switch level {
	case 5:
		return "Minimal access"
	case 10:
		return "Guest"
	case 20:
		return "Reporter"
	case 30:
		return "Developer"
	case 40:
		return "Maintainer"
	case 50:
		return "Owner"
	}
	return fmt.Sprintf("уровень %d", level)
}

// yourAccess renders the token's role on p as "Developer (30)", or "unknown"
// when GitLab returned no permissions.
func yourAccess(p *gitlab.Project) string {
	level := accessLevel(p)
	if level == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%s (%d)", roleName(level), level)
}

// isoDate formats a GitLab date as YYYY-MM-DD.
func isoDate(t *gitlab.ISOTime) string {
	return time.Time(*t).Format("2006-01-02")
}

// forbiddenReason names the exact reason a write to p is refused, or "" when
// nothing about the project explains it. The order is fixed: a project marked
// for deletion, an archived project, then a role below Developer.
func forbiddenReason(p *gitlab.Project) string {
	if p == nil {
		return ""
	}
	if p.MarkedForDeletionOn != nil {
		return fmt.Sprintf("проект запланирован к удалению (%s), запись невозможна", isoDate(p.MarkedForDeletionOn))
	}
	if p.Archived {
		return "проект в архиве, запись запрещена"
	}
	if level := accessLevel(p); level > 0 && level < developerLevel {
		return fmt.Sprintf("роль токена %s ниже Developer; нужен Developer+", roleName(level))
	}
	return ""
}

// explainForbidden looks up why a write was answered 403. For a write failure
// that carries its project it asks GitLab for the project and stores the exact
// reason on the error, which toToolText then prefers to the generic wording.
// The lookup is best-effort: any failure of it leaves err unchanged.
func (d Deps) explainForbidden(ctx context.Context, err error) error {
	var se *subjectError
	if !errors.As(err, &se) || !se.write || se.project == "" {
		return err
	}
	if e := glclient.Classify(err); e == nil || e.Kind != glclient.KindForbidden {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, forbiddenLookupTimeout)
	defer cancel()
	p, _, lookupErr := d.GL.Projects.GetProject(se.project, nil, gitlab.WithContext(ctx))
	if lookupErr != nil {
		return err
	}
	reason := forbiddenReason(p)
	if reason == "" {
		return err
	}
	cp := *se
	cp.reason = reason
	return &cp
}
