package glclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// maxRetries is the number of retries after the first attempt.
	maxRetries = 2
	// maxRetryWait caps a single wait; two waits give the 10 s total budget.
	maxRetryWait = 5 * time.Second
	// serverBackoff is the fixed wait before retrying a 5xx response.
	serverBackoff = 500 * time.Millisecond
)

// checkRetry is the retry policy of the shared client: only reads (GET/HEAD)
// are ever retried, so a failed write can never produce a duplicate commit,
// comment or merge request.
func checkRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if err != nil {
		// There is no response here; the method is the Op of the *url.Error.
		var ue *url.Error
		if errors.As(err, &ue) && (ue.Op == "Get" || ue.Op == "Head") {
			var dns *net.DNSError
			return !(errors.As(err, &dns) && dns.IsNotFound), nil
		}
		return false, nil
	}

	if resp == nil || resp.Request == nil {
		return false, nil
	}
	if m := resp.Request.Method; m != http.MethodGet && m != http.MethodHead {
		return false, nil
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		wait := retryAfter(resp)
		if wait > maxRetryWait {
			// Sleeping that long is pointless; the caller reports "retry in N s".
			return false, nil
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < wait+time.Second {
			return false, nil
		}
		return true, nil
	case resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented:
		return true, nil
	}
	return false, nil
}

// retryAfter parses an integer-seconds Retry-After header; it returns 0 when
// the header is absent or not a non-negative integer.
func retryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	v := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// backoff decides how long to wait before a retry. It honours Retry-After for
// 429 (client-go's built-in backoff reads RateLimit-Reset instead, which can
// sleep for minutes).
func backoff(_, _ time.Duration, _ int, resp *http.Response) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		wait := retryAfter(resp)
		if wait == 0 {
			wait = time.Second
		}
		if wait > maxRetryWait {
			wait = maxRetryWait
		}
		return wait
	}
	return serverBackoff
}

// noLimiter disables client-go's built-in rate limiter, which would otherwise
// block calls based on RateLimit-* response headers.
type noLimiter struct{}

// Wait returns immediately.
func (noLimiter) Wait(context.Context) error { return nil }
