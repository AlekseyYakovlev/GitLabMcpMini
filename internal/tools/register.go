package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_branches",
		Description: listBranchesDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, listBranches(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_commits",
		Description: listCommitsDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, listCommits(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_commit",
		Description: getCommitDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, getCommit(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "compare_refs",
		Description: compareRefsDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, safe(d, compareRefs(d)))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_branch",
		Description: createBranchDescription,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: gitlab.Ptr(false)},
	}, safe(d, createBranch(d)))
}
