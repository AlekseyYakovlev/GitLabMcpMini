package tools

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const listRepositoryTreeDescription = "Дерево файлов репозитория GitLab-проекта: одна строка на элемент. " +
	"Каталоги помечены «dir», файлы «file», подмодули «sub». " +
	"По умолчанию показывается корень репозитория на ветке по умолчанию; " +
	"чтобы заглянуть глубже, укажите path (каталог внутри репозитория), " +
	"а чтобы выбрать другую ветку, тег или коммит — ref. " +
	"С recursive=true выводятся все вложенные элементы с полными путями. " +
	"Вывод ограничен 15000 символами. " +
	"Для следующей страницы вызовите снова с page из подсказки внизу."

// TreeIn is the input of list_repository_tree.
type TreeIn struct {
	Project   string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Path      string `json:"path,omitempty" jsonschema:"directory inside the repository, default repository root"`
	Ref       string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA; default the project's default branch"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"list all nested entries; output is capped at 15000 characters"`
	Page      int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage   int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}

// listRepositoryTree returns the handler for the list_repository_tree tool.
func listRepositoryTree(d Deps) func(ctx context.Context, in TreeIn) (string, error) {
	return func(ctx context.Context, in TreeIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		path, err := glclient.NormalizeRepoPath(in.Path)
		if err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
		if err != nil {
			return "", err
		}

		opts := &gitlab.ListTreeOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			Ref:         gitlab.Ptr(ref),
		}
		if path != "" {
			opts.Path = gitlab.Ptr(path)
		}
		if in.Recursive {
			opts.Recursive = gitlab.Ptr(true)
		}

		nodes, resp, err := d.GL.Repositories.ListTree(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("путь или ref в дереве", err)
		}

		shownPath := path
		if shownPath == "" {
			shownPath = "/"
		}
		header := fmt.Sprintf("%s @ %s, путь: %s", project, refLabel(ref, isDefault), shownPath)
		if len(nodes) == 0 {
			return header + "\n(пусто)", nil
		}

		lines := make([]string, 0, len(nodes)+1)
		lines = append(lines, header)
		for _, n := range nodes {
			lines = append(lines, treeLine(n))
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// treeLine renders one tree entry with its full path so recursive listings
// stay unambiguous.
func treeLine(n *gitlab.TreeNode) string {
	switch n.Type {
	case "tree":
		return "dir  " + n.Path + "/"
	case "blob":
		return "file " + n.Path
	case "commit":
		return "sub  " + n.Path
	default:
		return n.Type + " " + n.Path
	}
}
