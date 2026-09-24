package tools

import (
	"strings"
	"testing"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestBudgetShortBodyUntouched(t *testing.T) {
	out, truncated := Budget("abc", 10)
	if out != "abc" || truncated {
		t.Fatalf("Budget = (%q, %v), want (abc, false)", out, truncated)
	}
}

func TestBudgetCutsOnLineBoundary(t *testing.T) {
	line := strings.Repeat("a", 999)
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, line)
	}
	body := strings.Join(lines, "\n") // 20 lines of 999 runes, 1000 with newline

	out, truncated := Budget(body, 15000)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	if n := utf8.RuneCountInString(out); n > 15000 {
		t.Fatalf("result has %d runes, want <= 15000", n)
	}
	got := strings.Split(out, "\n")
	if len(got) != 15 {
		t.Fatalf("kept %d lines, want 15", len(got))
	}
	for i, l := range got {
		if l != line {
			t.Fatalf("line %d is partial or altered (len %d)", i, len(l))
		}
	}
}

func TestBudgetLinesLongerThanPeriod(t *testing.T) {
	line := strings.Repeat("a", 1000)
	body := strings.TrimSuffix(strings.Repeat(line+"\n", 20), "\n")
	out, truncated := Budget(body, 15000)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	got := strings.Split(out, "\n")
	if len(got) != 14 {
		t.Fatalf("kept %d lines, want 14 full lines", len(got))
	}
	for i, l := range got {
		if l != line {
			t.Fatalf("line %d is partial (len %d)", i, len(l))
		}
	}
}

func TestBudgetSingleLongLineHardCut(t *testing.T) {
	out, truncated := Budget(strings.Repeat("x", 20000), 15000)
	if !truncated {
		t.Fatalf("expected truncation")
	}
	if n := utf8.RuneCountInString(out); n != 15000 {
		t.Fatalf("got %d runes, want exactly 15000", n)
	}
}

func TestBudgetCountsRunesNotBytes(t *testing.T) {
	body := strings.Repeat("я", 15000) // 30000 bytes
	out, truncated := Budget(body, 15000)
	if truncated || out != body {
		t.Fatalf("15000 Cyrillic runes must fit: truncated=%v len=%d", truncated, len(out))
	}
}

func TestClampPaging(t *testing.T) {
	cases := []struct{ page, per, wantPage, wantPer int }{
		{0, 0, 1, 20},
		{3, 500, 3, 100},
		{-1, 5, 1, 5},
		{2, 100, 2, 100},
		{1, 1, 1, 1},
	}
	for _, c := range cases {
		p, pp := ClampPaging(c.page, c.per)
		if p != c.wantPage || pp != c.wantPer {
			t.Errorf("ClampPaging(%d,%d) = (%d,%d), want (%d,%d)", c.page, c.per, p, pp, c.wantPage, c.wantPer)
		}
	}
}

func TestPageFooter(t *testing.T) {
	const wantNext = "[page 1, per_page 20 — есть следующая страница: вызовите с page=2]"
	if got := PageFooter(1, 20, &gitlab.Response{NextPage: 2}); got != wantNext {
		t.Errorf("next page footer = %q, want %q", got, wantNext)
	}

	const wantLast2 = "[page 2, per_page 20 — последняя страница]"
	if got := PageFooter(2, 20, &gitlab.Response{}); got != wantLast2 {
		t.Errorf("last page footer = %q, want %q", got, wantLast2)
	}

	if got := PageFooter(1, 20, &gitlab.Response{NextLink: "https://x?page=2"}); got != wantNext {
		t.Errorf("NextLink footer = %q, want %q", got, wantNext)
	}

	const wantLast1 = "[page 1, per_page 20 — последняя страница]"
	if got := PageFooter(1, 20, nil); got != wantLast1 {
		t.Errorf("nil response footer = %q, want %q", got, wantLast1)
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("first\nsecond", 100); got != "first" {
		t.Errorf("oneLine first line = %q, want first", got)
	}
	if got := oneLine("  padded  ", 100); got != "padded" {
		t.Errorf("oneLine trim = %q, want padded", got)
	}
	got := oneLine(strings.Repeat("ж", 200), 120)
	if n := utf8.RuneCountInString(got); n != 120 || !strings.HasSuffix(got, "…") {
		t.Errorf("oneLine long = %d runes, suffix ok=%v; want 120 ending with …", n, strings.HasSuffix(got, "…"))
	}
}

func TestComposeListOrdersFooters(t *testing.T) {
	line := strings.Repeat("z", 999)
	body := strings.TrimSuffix(strings.Repeat(line+"\n", 20), "\n")
	got := composeList(body, 1, 20, &gitlab.Response{NextPage: 2})
	lines := strings.Split(got, "\n")
	if lines[len(lines)-2] != TruncatedFooter {
		t.Errorf("second to last line = %q, want truncation footer", lines[len(lines)-2])
	}
	if !strings.Contains(lines[len(lines)-1], "page=2") {
		t.Errorf("last line = %q, want next-page footer", lines[len(lines)-1])
	}
}
