package glclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// maxDetailRunes caps any GitLab- or network-supplied text kept in an Error.
const maxDetailRunes = 300

// Kind is the failure class of a GitLab call.
type Kind int

// Failure classes. The zero value is KindOther.
const (
	KindOther Kind = iota
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindRateLimited
	KindServer
	KindBadRequest
	KindNetwork
	KindTimeout
	KindCanceled
	KindTooLarge
	KindDecode
)

// Error is a classified GitLab failure. Tools turn it into a short message;
// it carries no raw response bodies except a capped Detail for client errors.
type Error struct {
	Kind       Kind
	Status     int
	RetryAfter time.Duration
	Detail     string
}

func (e *Error) Error() string {
	switch {
	case e.Status != 0 && e.Detail != "":
		return fmt.Sprintf("gitlab: kind %d, status %d: %s", e.Kind, e.Status, e.Detail)
	case e.Status != 0:
		return fmt.Sprintf("gitlab: kind %d, status %d", e.Kind, e.Status)
	case e.Detail != "":
		return fmt.Sprintf("gitlab: kind %d: %s", e.Kind, e.Detail)
	}
	return fmt.Sprintf("gitlab: kind %d", e.Kind)
}

// Classify maps an error returned by client-go (or by the transport) to an
// *Error. It returns nil for a nil error.
func Classify(err error) *Error {
	if err == nil {
		return nil
	}

	var already *Error
	if errors.As(err, &already) {
		return already
	}

	switch {
	case errors.Is(err, context.Canceled):
		return &Error{Kind: KindCanceled}
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Kind: KindTimeout}
	case errors.Is(err, ErrBodyTooLarge):
		return &Error{Kind: KindTooLarge}
	}

	var er *gitlab.ErrorResponse
	if errors.As(err, &er) {
		return classifyResponse(er)
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return &Error{Kind: KindDecode}
	}

	var ue *url.Error
	if errors.As(err, &ue) {
		return &Error{Kind: KindNetwork, Detail: capRunes(ue.Err.Error())}
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return &Error{Kind: KindNetwork, Detail: capRunes(err.Error())}
	}

	return &Error{Kind: KindOther, Detail: capRunes(err.Error())}
}

func classifyResponse(er *gitlab.ErrorResponse) *Error {
	status := er.StatusCode
	if status == 0 && er.Response != nil {
		status = er.Response.StatusCode
	}
	out := &Error{Status: status, Detail: capRunes(er.Message)}

	switch {
	case status == http.StatusUnauthorized:
		out.Kind = KindUnauthorized
	case status == http.StatusForbidden:
		out.Kind = KindForbidden
	case status == http.StatusNotFound:
		out.Kind = KindNotFound
	case status == http.StatusTooManyRequests:
		out.Kind = KindRateLimited
		if er.Response != nil {
			out.RetryAfter = retryAfter(er.Response)
		}
	case status >= 500:
		// Server error bodies are often HTML pages; never echo them.
		out.Kind = KindServer
		out.Detail = ""
	case status >= 400:
		out.Kind = KindBadRequest
	default:
		out.Kind = KindOther
	}
	return out
}

// capRunes truncates s to maxDetailRunes runes.
func capRunes(s string) string {
	r := []rune(s)
	if len(r) <= maxDetailRunes {
		return s
	}
	return string(r[:maxDetailRunes])
}
