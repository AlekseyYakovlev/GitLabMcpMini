// Command gitlab-mcp is a stdio MCP server exposing a compact set of GitLab tools.
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/config"
	"gitlab-mcp/internal/glclient"
	"gitlab-mcp/internal/logging"
	"gitlab-mcp/internal/server"
	"gitlab-mcp/internal/tools"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		_, _ = os.Stderr.WriteString("gitlab-mcp: " + err.Error() + "\n")
		return 1
	}

	// Stdout carries only JSON-RPC. Keep the real handle for the transport and
	// point os.Stdout at stderr so a stray write can never corrupt the stream.
	realOut := os.Stdout
	os.Stdout = os.Stderr

	redact := logging.NewRedactor(cfg.Token)
	logger := logging.New(os.Stderr, cfg.LogLevel, redact)
	slog.SetDefault(logger)
	log.SetOutput(os.Stderr)

	gl, err := glclient.New(cfg, logger)
	if err != nil {
		_, _ = os.Stderr.WriteString("gitlab-mcp: " + redact(err.Error()) + "\n")
		return 1
	}

	srv := server.New(tools.Deps{GL: gl, Logger: logger, Redact: redact, Timeout: tools.CallTimeout})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err = srv.Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: realOut})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("server stopped with error", "err", err)
		return 1
	}
	return 0
}
