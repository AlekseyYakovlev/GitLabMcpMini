package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds every tool to the server.
func Register(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "whoami",
		Description: whoamiDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, whoami(d)))
}
