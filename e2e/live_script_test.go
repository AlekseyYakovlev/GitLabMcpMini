package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveScriptUsage checks the opt-in gate of scripts/live.py without any
// network access: the script must refuse to start without --project or without
// a token. It never passes a token, because a real run writes to gitlab.com.
func TestLiveScriptUsage(t *testing.T) {
	uv, err := exec.LookPath("uv")
	if err != nil {
		t.Skip("uv is not on PATH; skipping the live.py usage test")
	}

	run := func(args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(uv, append([]string{"run", "scripts/live.py"}, args...)...)
		cmd.Dir = ".."
		cmd.Env = envWithout("GITLAB_TOKEN", "GITLAB_TOKEN_READONLY", "GITLAB_URL")
		out, err := cmd.CombinedOutput()
		if err == nil {
			return string(out), 0
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("uv run scripts/live.py %v: %v\n%s", args, err, out)
		}
		return string(out), exitErr.ExitCode()
	}

	t.Run("no arguments", func(t *testing.T) {
		out, code := run()
		if code != 2 {
			t.Fatalf("exit code = %d, want 2\n%s", code, out)
		}
	})

	t.Run("no token", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "LIVE-RUN.md")
		out, code := run("--project", "g/p", "--out", transcript)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2\n%s", code, out)
		}
		if !strings.Contains(out, "GITLAB_TOKEN") {
			t.Errorf("output does not name GITLAB_TOKEN:\n%s", out)
		}
		if _, err := os.Stat(transcript); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("transcript %s exists after a usage error (stat err = %v)", transcript, err)
		}
	})

	t.Run("help", func(t *testing.T) {
		out, code := run("--help")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", code, out)
		}
		if !strings.Contains(out, "--project") {
			t.Errorf("--help does not mention --project:\n%s", out)
		}
	})
}
