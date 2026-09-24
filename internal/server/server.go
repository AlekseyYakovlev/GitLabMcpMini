// Package server assembles the MCP server.
package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/tools"
)

// New creates the MCP server with all tools registered. Running it is left to
// the caller, which owns the transport.
func New(d tools.Deps) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "gitlab-mcp", Version: "0.1.0"},
		&mcp.ServerOptions{Logger: d.Logger},
	)
	tools.Register(s, d)
	return s
}
