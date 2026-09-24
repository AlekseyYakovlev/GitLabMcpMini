// Output formatting shared by every list-style tool.
//
// Composition rule: body lines first; if the body was cut by Budget, append
// TruncatedFooter on its own line; then append the page footer on its own
// line. Footers are added after budgeting, so OutputBudget plus footers stays
// well under the 20 000 character cut applied by the agent.
//
// Pagination state comes only from X-Next-Page / the Link header. Total
// counters are never read: GitLab omits them for large result sets.
package tools

import (
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	// OutputBudget is the maximum number of characters (runes) of tool output
	// body before truncation.
	OutputBudget = 15000

	// TruncatedFooter marks output cut by Budget.
	TruncatedFooter = "[вывод обрезан на 15000 символах; сузьте путь или уменьшите per_page]"

	defaultPerPage = 20
	maxPerPage     = 100
)

// Budget cuts body to at most limit runes. Runes are counted, not bytes,
// because the agent cuts by Python len(str). When a newline exists in the kept
// part, the cut is made on the last line boundary so no partial line remains
// (the newline itself is dropped); otherwise the cut is a hard rune cut.
func Budget(body string, limit int) (out string, truncated bool) {
	runes := []rune(body)
	if len(runes) <= limit {
		return body, false
	}
	if runes[limit] == '\n' {
		return string(runes[:limit]), true
	}
	kept := runes[:limit]
	for i := len(kept) - 1; i >= 0; i-- {
		if kept[i] == '\n' {
			return string(kept[:i]), true
		}
	}
	return string(kept), true
}

// ClampPaging normalises page and per_page arguments: page < 1 becomes 1,
// per_page < 1 becomes 20 and per_page > 100 becomes 100.
func ClampPaging(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

// PageFooter renders the pagination hint. A next page exists when GitLab sent
// X-Next-Page or a Link rel=next.
func PageFooter(page, perPage int, resp *gitlab.Response) string {
	if resp != nil && (resp.NextPage > 0 || resp.NextLink != "") {
		next := int(resp.NextPage)
		if next <= 0 {
			next = page + 1
		}
		return fmt.Sprintf("[page %d, per_page %d — есть следующая страница: вызовите с page=%d]", page, perPage, next)
	}
	return fmt.Sprintf("[page %d, per_page %d — последняя страница]", page, perPage)
}

// composeList applies the composition rule from the package comment to the
// body lines of a paginated list.
func composeList(body string, page, perPage int, resp *gitlab.Response) string {
	out, truncated := Budget(body, OutputBudget)
	var sb strings.Builder
	sb.WriteString(out)
	if truncated {
		sb.WriteString("\n")
		sb.WriteString(TruncatedFooter)
	}
	sb.WriteString("\n")
	sb.WriteString(PageFooter(page, perPage, resp))
	return sb.String()
}

// oneLine returns the first line of s, trimmed, capped at max runes with a
// trailing "…" when it was shortened.
func oneLine(s string, max int) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	r := []rune(s)
	if max > 0 && len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
