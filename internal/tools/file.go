package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const getFileContentsDescription = "Содержимое текстового файла из репозитория GitLab-проекта. " +
	"Без ref читается ветка по умолчанию. " +
	"Для больших файлов используйте start_line и end_line (нумерация с 1, обе границы включительно) — " +
	"так можно читать файл по частям. " +
	"Без диапазона выводится не больше 15000 символов; при обрезке в конце указано, " +
	"какие строки показаны и как запросить остальное. " +
	"Заголовок содержит путь, ref, диапазон строк, общее число строк и размер. " +
	"Бинарные файлы (изображения, архивы и т. п.) не выводятся: вместо содержимого " +
	"возвращается пометка с размером и blob_id."

// binaryProbeBytes is how many leading bytes are inspected to tell binary
// content from text.
const binaryProbeBytes = 8000

// utf8BOM is the UTF-8 byte order mark that some editors put at file start.
const utf8BOM = "\xEF\xBB\xBF"

// FileIn is the input of get_file_contents.
type FileIn struct {
	Project   string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Path      string `json:"path" jsonschema:"file path inside the repository, for example src/main.go"`
	Ref       string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA; default the project's default branch"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line to return, 1-based; use for big files"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"last line to return, inclusive"`
}

// getFileContents returns the handler for the get_file_contents tool.
func getFileContents(d Deps) func(ctx context.Context, in FileIn) (string, error) {
	return func(ctx context.Context, in FileIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		path, err := glclient.NormalizeRepoPath(in.Path)
		if err != nil {
			return "", err
		}
		if path == "" {
			return "", errors.New("не указан путь к файлу")
		}

		ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
		if err != nil {
			return "", err
		}

		f, _, err := d.GL.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(ref)}, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("файл", err)
		}

		var raw []byte
		if f.Encoding == "base64" {
			raw, err = base64.StdEncoding.DecodeString(f.Content)
			if err != nil {
				return "", fmt.Errorf("не удалось декодировать содержимое файла: %w", err)
			}
		} else {
			raw = []byte(f.Content)
		}

		label := path + " @ " + refLabel(ref, isDefault)
		if isBinary(raw) {
			return fmt.Sprintf("%s: файл бинарный, %d байт, blob_id %s", label, f.Size, f.BlobID), nil
		}

		text := strings.TrimPrefix(string(raw), utf8BOM)
		lines, from, to, total, err := selectLines(text, in.StartLine, in.EndLine)
		if err != nil {
			return "", err
		}
		if total == 0 {
			return fmt.Sprintf("%s — файл пуст, %d байт", label, f.Size), nil
		}

		header := fmt.Sprintf("%s — строки %d-%d из %d, %d байт", label, from, to, total, f.Size)
		body, truncated := Budget(strings.Join(lines, "\n"), OutputBudget)

		var sb strings.Builder
		sb.WriteString(header)
		sb.WriteString("\n")
		sb.WriteString(body)
		if truncated {
			shown := 0
			if body != "" {
				shown = strings.Count(body, "\n") + 1
			}
			fmt.Fprintf(&sb, "\n[файл обрезан на 15000 символах: показаны строки %d-%d из %d; используйте start_line/end_line]",
				from, from+shown-1, total)
		}
		return sb.String(), nil
	}
}

// isBinary reports whether b looks like binary content: a NUL byte, or bytes
// that are not valid UTF-8, within the first 8000 bytes. A probe that cuts a
// multi-byte rune in half is not treated as invalid.
func isBinary(b []byte) bool {
	probe := b
	cut := false
	if len(probe) > binaryProbeBytes {
		probe = probe[:binaryProbeBytes]
		cut = true
	}
	for _, c := range probe {
		if c == 0 {
			return true
		}
	}
	if cut {
		// Step back over an incomplete trailing rune (at most 3 bytes).
		for i := 1; i <= utf8.UTFMax-1 && i <= len(probe); i++ {
			tail := probe[len(probe)-i:]
			if utf8.RuneStart(tail[0]) {
				if !utf8.FullRune(tail) {
					probe = probe[:len(probe)-i]
				}
				break
			}
		}
	}
	return !utf8.Valid(probe)
}

// selectLines splits text into lines (a trailing "\r" is dropped from each
// line) and returns the inclusive 1-based range [start, end]. A zero start or
// end means "unset". end is clamped to the number of lines; a start beyond the
// end of the file or an end before the start is an error.
func selectLines(text string, start, end int) (lines []string, from, to, total int, err error) {
	var all []string
	if text != "" {
		all = strings.Split(text, "\n")
		if all[len(all)-1] == "" {
			all = all[:len(all)-1]
		}
		for i, l := range all {
			all[i] = strings.TrimSuffix(l, "\r")
		}
	}
	total = len(all)

	if start < 0 || end < 0 {
		return nil, 0, 0, total, errors.New("start_line и end_line должны быть не меньше 1")
	}
	if total == 0 && start == 0 && end == 0 {
		return nil, 0, 0, 0, nil
	}
	if start == 0 {
		start = 1
	}
	if start > total {
		return nil, 0, 0, total, fmt.Errorf("start_line %d за пределами файла: в файле %d строк", start, total)
	}
	if end == 0 || end > total {
		end = total
	}
	if end < start {
		return nil, 0, 0, total, fmt.Errorf("end_line %d меньше start_line %d", end, start)
	}
	return all[start-1 : end], start, end, total, nil
}
