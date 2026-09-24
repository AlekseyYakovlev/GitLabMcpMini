// Package glclient builds the shared GitLab REST client.
package glclient

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/config"
)

// defaultMaxBody is the largest response body the client will read.
const defaultMaxBody = 8 << 20

// limits are the tunable safety limits of the client; tests lower them.
type limits struct {
	maxBody int64
}

// New builds the one GitLab client used by every tool. It performs no network
// I/O: client-go authenticates lazily on the first request.
//
// Retry policy: only GET and HEAD requests are retried (at most twice, waiting
// at most 10 s in total). POST, PUT and DELETE are sent exactly once whatever
// the response, so writes are never repeated automatically.
func New(cfg config.Config, logger *slog.Logger) (*gitlab.Client, error) {
	return newClient(cfg, logger, limits{maxBody: defaultMaxBody})
}

func newClient(cfg config.Config, logger *slog.Logger, l limits) (*gitlab.Client, error) {
	return gitlab.NewClient(cfg.Token, clientOptions(cfg, logger, newHTTPClient(l))...)
}

// clientOptions lists the client-go options applied to the shared client.
func clientOptions(cfg config.Config, logger *slog.Logger, hc *http.Client) []gitlab.ClientOptionFunc {
	return []gitlab.ClientOptionFunc{
		gitlab.WithBaseURL(cfg.BaseURL),
		gitlab.WithHTTPClient(hc),
		gitlab.WithUserAgent("gitlab-mcp/0.1"),
		gitlab.WithURLWarningLogger(logger),
		gitlab.WithCustomRetry(retryablehttp.CheckRetry(checkRetry)),
		gitlab.WithCustomBackoff(backoff),
		gitlab.WithCustomRetryMax(maxRetries),
		gitlab.WithCustomLimiter(noLimiter{}),
	}
}

// newHTTPClient returns an HTTP client with bounded timeouts at every stage
// and a cap on the response body size.
func newHTTPClient(l limits) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: capRT{
			rt: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				IdleConnTimeout:       30 * time.Second,
			},
			max: l.maxBody,
		},
	}
}
