// Package glclient builds the shared GitLab REST client.
package glclient

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/config"
)

// New builds the one GitLab client used by every tool. It performs no network
// I/O: client-go authenticates lazily on the first request.
func New(cfg config.Config, logger *slog.Logger) (*gitlab.Client, error) {
	return gitlab.NewClient(cfg.Token, clientOptions(cfg, logger, newHTTPClient())...)
}

// clientOptions lists the client-go options applied to the shared client.
func clientOptions(cfg config.Config, logger *slog.Logger, hc *http.Client) []gitlab.ClientOptionFunc {
	return []gitlab.ClientOptionFunc{
		gitlab.WithBaseURL(cfg.BaseURL),
		gitlab.WithHTTPClient(hc),
		gitlab.WithUserAgent("gitlab-mcp/0.1"),
		gitlab.WithURLWarningLogger(logger),
	}
}

// newHTTPClient returns an HTTP client with bounded timeouts at every stage.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}
