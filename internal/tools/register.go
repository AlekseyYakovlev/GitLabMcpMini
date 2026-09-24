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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: listProjectsDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, listProjects(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_project",
		Description: getProjectDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, getProject(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_repository_tree",
		Description: listRepositoryTreeDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, listRepositoryTree(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_file_contents",
		Description: getFileContentsDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, getFileContents(d)))
}
