package glclient

import (
	"errors"
	"net/url"
	"strings"
)

// Errors returned by the normalisers. Their texts reach the model as-is.
var (
	ErrProjectURL   = errors.New("укажите проект числовым ID или путём group/subgroup/project, а не URL")
	ErrEmptyProject = errors.New("не указан проект")
	ErrDotDot       = errors.New("путь не может содержать '..'")
)

// NormalizeProject cleans a project reference (numeric ID or
// group/subgroup/project path). It only fixes the meaning of the input and
// never percent-encodes it: client-go encodes the value exactly once when it
// builds the request, so encoding here would double-encode.
func NormalizeProject(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return "", ErrProjectURL
	}
	// A project path cannot contain '%', so unescaping a model-supplied
	// "group%2Fproj" is safe.
	if strings.Contains(s, "%") {
		if u, err := url.PathUnescape(s); err == nil {
			s = u
		}
	}
	s = strings.Trim(s, "/")
	if s == "" {
		return "", ErrEmptyProject
	}
	return s, nil
}

// NormalizeRepoPath cleans a repository path used as a tree filter or a file
// path. The empty string means the repository root. Like NormalizeProject it
// leaves encoding to client-go.
func NormalizeRepoPath(p string) (string, error) {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", ErrDotDot
		}
	}
	return p, nil
}
