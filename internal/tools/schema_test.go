package tools

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

const (
	maxToolNameLen        = 30
	maxDescriptionRunes   = 900
	forbiddenNamePrefix   = "gitlab_"
	minRegisteredToolsReq = 1
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// forbiddenSchemaKeys are JSON Schema constructs that some MCP clients cannot
// digest; tool schemas must stay flat.
var forbiddenSchemaKeys = []string{"$ref", "$defs", "anyOf", "oneOf", "allOf"}

// TestToolSchemas checks every registered tool, so tools added later are
// covered automatically.
func TestToolSchemas(t *testing.T) {
	cs := newTestSession(t, testutil.NewFakeGitLab(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(list.Tools) < minRegisteredToolsReq {
		t.Fatalf("no tools registered")
	}

	for _, tool := range list.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			if len(tool.Name) > maxToolNameLen {
				t.Errorf("name %q is %d chars, limit %d", tool.Name, len(tool.Name), maxToolNameLen)
			}
			if !toolNamePattern.MatchString(tool.Name) {
				t.Errorf("name %q does not match %s", tool.Name, toolNamePattern)
			}
			if strings.HasPrefix(tool.Name, forbiddenNamePrefix) {
				t.Errorf("name %q must not carry the %q prefix", tool.Name, forbiddenNamePrefix)
			}

			// Characters, not bytes: Cyrillic is 2 bytes per character and the
			// limit is defined in characters (same as Python len(str)).
			n := utf8.RuneCountInString(tool.Description)
			if n == 0 || n > maxDescriptionRunes {
				t.Errorf("description length %d chars, want 1..%d", n, maxDescriptionRunes)
			}

			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("marshal InputSchema: %v", err)
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatalf("decode InputSchema: %v", err)
			}
			if schema["type"] != "object" {
				t.Errorf("schema type = %v, want object (schema %s)", schema["type"], raw)
			}
			walkSchema(t, "$", schema)

			props, _ := schema["properties"].(map[string]any)
			for name, p := range props {
				prop, _ := p.(map[string]any)
				if d, _ := prop["description"].(string); strings.TrimSpace(d) == "" {
					t.Errorf("property %q has no description", name)
				}
			}
		})
	}
}

// walkSchema fails on forbidden keywords and on array-valued "type" (how
// pointer fields render as ["null", ...]) anywhere in the schema tree.
func walkSchema(t *testing.T, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for _, k := range forbiddenSchemaKeys {
			if _, ok := x[k]; ok {
				t.Errorf("%s: forbidden schema keyword %q", path, k)
			}
		}
		if typ, ok := x["type"]; ok {
			if _, isArr := typ.([]any); isArr {
				t.Errorf("%s: type is an array %v, use a plain type", path, typ)
			}
		}
		for k, child := range x {
			walkSchema(t, path+"."+k, child)
		}
	case []any:
		for i, child := range x {
			walkSchema(t, fmt.Sprintf("%s[%d]", path, i), child)
		}
	}
}

// updateGolden rewrites the golden files under testdata instead of comparing.
var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

const toolsListGolden = "testdata/tools_list.golden.json"

// TestToolsListGolden pins the exact tools/list answer, so any change to a tool
// name, description or input schema is a visible diff.
func TestToolsListGolden(t *testing.T) {
	cs := newTestSession(t, testutil.NewFakeGitLab(t))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got, err := json.MarshalIndent(list.Tools, "", "  ")
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	got = append(got, '\n')

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(toolsListGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(toolsListGolden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(toolsListGolden)
	if err != nil {
		t.Fatalf("read golden: %v (run `go test ./internal/tools -run TestToolsListGolden -update`)", err)
	}
	norm := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
	if norm(got) != norm(want) {
		t.Errorf("tools/list differs from %s; if the change is intended run `go test ./internal/tools -run TestToolsListGolden -update`\n--- got ---\n%s", toolsListGolden, got)
	}
}
