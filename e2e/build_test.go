package e2e

import (
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// maxExeSize is the upper bound for the release binary (about 11 MB today).
const maxExeSize = 25 << 20

// TestBinaryHasNoExternalDependencies builds the documented release binary and
// checks that it is a single self-contained exe: only kernel32.dll is
// imported, cgo is off, and the MCP and GitLab client modules are embedded.
func TestBinaryHasNoExternalDependencies(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PE import check is Windows-only")
	}

	out := filepath.Join(t.TempDir(), "gitlab-mcp.exe")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "./cmd/gitlab-mcp")
	build.Dir = ".."
	build.Env = append(envWithout("CGO_ENABLED"), "CGO_ENABLED=0")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("canonical build failed: %v\n%s", err, b)
	}

	f, err := pe.Open(out)
	if err != nil {
		t.Fatalf("open PE: %v", err)
	}
	defer f.Close()

	syms, err := f.ImportedSymbols()
	if err != nil {
		t.Fatalf("imported symbols: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("no PE imports found; the check would be vacuous")
	}
	for _, s := range syms {
		i := strings.LastIndex(s, ":")
		dll := strings.ToLower(s[i+1:])
		if dll != "kernel32.dll" {
			t.Errorf("unexpected import %q (dll %q); only kernel32.dll is allowed", s, dll)
		}
	}

	info, err := exec.Command("go", "version", "-m", out).CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m: %v\n%s", err, info)
	}
	text := string(info)
	for _, want := range []string{
		"CGO_ENABLED=0",
		"-trimpath=true",
		"dep\tgithub.com/modelcontextprotocol/go-sdk",
		"dep\tgitlab.com/gitlab-org/api/client-go/v2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("go version -m output lacks %q:\n%s", want, text)
		}
	}

	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	t.Logf("size %d bytes, %d imported symbols", st.Size(), len(syms))
	if st.Size() >= maxExeSize {
		t.Errorf("binary is %d bytes, want < %d", st.Size(), maxExeSize)
	}
}
