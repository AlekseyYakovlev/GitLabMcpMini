package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a bytes.Buffer that is safe for the concurrent writes the
// os/exec stderr copier performs while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// stdoutChunk is one newline-terminated read from the server's stdout, or the
// final read (possibly with leftover bytes) together with the terminating error.
type stdoutChunk struct {
	data []byte
	err  error
}

const purityStepTimeout = 10 * time.Second

// TestStdoutPurity drives the real binary over raw pipes and checks that
// stdout carries nothing but JSON-RPC 2.0 lines, that starting up and listing
// tools performs no network I/O, that the token never appears in stdout or in
// debug-level stderr, and that closing stdin makes the process exit cleanly.
func TestStdoutPurity(t *testing.T) {
	fake := newFakeWithWhoami(t)
	fake.JSON("GET", "/api/v4/user", 401, `{"message":"401 Unauthorized"}`, nil)

	cmd := exec.Command(binPath)
	cmd.Env = append(envWithout("GITLAB_TOKEN", "GITLAB_URL", "LOG_LEVEL"),
		"GITLAB_TOKEN="+testToken, "GITLAB_URL="+fake.URL, "LOG_LEVEL=debug")
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Never leave the process running: a locked exe breaks temp cleanup on Windows.
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
	})

	chunks := make(chan stdoutChunk, 64)
	go func() {
		defer close(chunks)
		r := bufio.NewReader(stdoutPipe)
		for {
			line, err := r.ReadBytes('\n')
			chunks <- stdoutChunk{data: line, err: err}
			if err != nil {
				return
			}
		}
	}()

	var allStdout bytes.Buffer

	// next returns the next stdout line, validated as a JSON-RPC 2.0 message.
	next := func(step string) map[string]any {
		t.Helper()
		select {
		case c, ok := <-chunks:
			if !ok {
				t.Fatalf("%s: stdout closed unexpectedly; stderr: %s", step, stderr.String())
			}
			allStdout.Write(c.data)
			if c.err != nil {
				t.Fatalf("%s: stdout read error %v with data %q; stderr: %s", step, c.err, c.data, stderr.String())
			}
			return checkRPCLine(t, step, c.data)
		case <-time.After(purityStepTimeout):
			t.Fatalf("%s: no stdout line within %s; stderr: %s", step, purityStepTimeout, stderr.String())
			return nil
		}
	}

	// awaitResponse reads lines until the response with the given id arrives.
	// Server notifications on the way are validated but otherwise ignored.
	awaitResponse := func(step string, id int) map[string]any {
		t.Helper()
		for {
			msg := next(step)
			if got, ok := msg["id"]; ok && msg["method"] == nil {
				if gotID, _ := got.(float64); int(gotID) != id {
					t.Fatalf("%s: response id = %v, want %d", step, got, id)
				}
				return msg
			}
		}
	}

	send := func(v map[string]any) {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stdin.Write(append(b, '\n')); err != nil {
			t.Fatalf("write to stdin: %v; stderr: %s", err, stderr.String())
		}
	}
	rpc := func(id int, method string, params map[string]any) {
		t.Helper()
		send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	}
	callTool := func(id int, name string, args map[string]any) {
		t.Helper()
		rpc(id, "tools/call", map[string]any{"name": name, "arguments": args})
	}

	// 1: initialize
	rpc(1, "initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "purity", "version": "1"},
	})
	if msg := awaitResponse("initialize", 1); msg["result"] == nil {
		t.Fatalf("initialize returned no result: %v", msg)
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Fatalf("network requests after initialize: %v", reqs)
	}

	// 2: tools/list
	rpc(2, "tools/list", map[string]any{})
	if msg := awaitResponse("tools/list", 2); msg["result"] == nil {
		t.Fatalf("tools/list returned no result: %v", msg)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Fatalf("network requests after tools/list: %v", reqs)
	}

	// 3: whoami against a 401 must be a tool error, not a protocol failure.
	callTool(3, "whoami", nil)
	msg := awaitResponse("whoami 401", 3)
	result, _ := msg["result"].(map[string]any)
	if result == nil || result["isError"] != true {
		t.Fatalf("whoami with 401 = %v, want result.isError true", msg)
	}
	if text := fmt.Sprint(result["content"]); !strings.Contains(text, "401") {
		t.Errorf("whoami 401 text %q does not mention 401", text)
	}

	// 4: unknown tool, 5: invalid arguments. Either a JSON-RPC error or an
	// isError result is acceptable; a broken stream is not.
	callTool(4, "no_such_tool", map[string]any{})
	if msg := awaitResponse("unknown tool", 4); msg["result"] == nil && msg["error"] == nil {
		t.Errorf("unknown tool: neither result nor error: %v", msg)
	}
	callTool(5, "whoami", map[string]any{"bogus": 1})
	if msg := awaitResponse("invalid arguments", 5); msg["result"] == nil && msg["error"] == nil {
		t.Errorf("invalid arguments: neither result nor error: %v", msg)
	}

	// Closing stdin must end the process with exit code 0 and no further stdout.
	if err := stdin.Close(); err != nil {
		t.Logf("close stdin: %v", err)
	}
	select {
	case c, ok := <-chunks:
		if ok {
			allStdout.Write(c.data)
			if len(c.data) > 0 || c.err != io.EOF {
				t.Errorf("stdout after stdin EOF: data %q err %v, want clean EOF", c.data, c.err)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("stdout not closed within 2 s after stdin EOF; stderr: %s", stderr.String())
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		if err != nil {
			t.Errorf("process exit after stdin EOF: %v, want exit code 0; stderr: %s", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("process did not exit within 2 s after stdin EOF; stderr: %s", stderr.String())
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	if strings.Contains(allStdout.String(), testToken) {
		t.Errorf("stdout leaks the token: %q", allStdout.String())
	}
	if strings.Contains(stderr.String(), testToken) {
		t.Errorf("stderr leaks the token at LOG_LEVEL=debug: %q", stderr.String())
	}
}

// checkRPCLine asserts a raw stdout line is one well-formed JSON-RPC 2.0
// message: newline-terminated, no carriage return, valid JSON object.
func checkRPCLine(t *testing.T, step string, line []byte) map[string]any {
	t.Helper()
	if bytes.ContainsRune(line, '\r') {
		t.Errorf("%s: stdout line contains \\r: %q", step, line)
	}
	if !bytes.HasSuffix(line, []byte("\n")) {
		t.Errorf("%s: stdout line not newline-terminated: %q", step, line)
	}
	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimRight(line, "\r\n"), &msg); err != nil {
		t.Fatalf("%s: stdout line is not a JSON object: %q (%v)", step, line, err)
	}
	if msg["jsonrpc"] != "2.0" {
		t.Errorf("%s: jsonrpc = %v, want \"2.0\": %q", step, msg["jsonrpc"], line)
	}
	_, hasID := msg["id"]
	_, hasResult := msg["result"]
	_, hasError := msg["error"]
	_, hasMethod := msg["method"]
	switch {
	case hasMethod:
		// request or notification from the server
	case hasID && (hasResult || hasError):
		// response
	default:
		t.Errorf("%s: line is neither a response nor a notification: %q", step, line)
	}
	return msg
}
