package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const whoamiDescription = "Показывает текущего пользователя GitLab по токену и scopes токена. " +
	"Используйте для быстрой проверки токена и прав."

// whoamiIn has no arguments: the inferred schema is an empty closed object.
type whoamiIn struct{}

// whoami returns the handler for the whoami tool.
func whoami(d Deps) func(ctx context.Context, in whoamiIn) (string, error) {
	return func(ctx context.Context, _ whoamiIn) (string, error) {
		user, _, err := d.GL.Users.CurrentUser(gitlab.WithContext(ctx))
		if err != nil {
			return "", err
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "@%s (%s), id %d, state %s, %s\n",
			user.Username, user.Name, user.ID, user.State, user.WebURL)
		sb.WriteString(tokenLine(ctx, d))
		return sb.String(), nil
	}
}

// tokenLine describes the token scopes and expiry. It is best effort: any
// failure yields a fixed line and never fails the tool call.
func tokenLine(ctx context.Context, d Deps) string {
	tok, _, err := d.GL.PersonalAccessTokens.GetSinglePersonalAccessToken(gitlab.WithContext(ctx))
	if err != nil || tok == nil {
		return "token: scopes unavailable"
	}
	expires := "never"
	if tok.ExpiresAt != nil {
		expires = time.Time(*tok.ExpiresAt).Format("2006-01-02")
	}
	return fmt.Sprintf("token: scopes [%s], expires %s", strings.Join(tok.Scopes, ", "), expires)
}
