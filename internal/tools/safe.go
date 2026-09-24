// Package tools implements the MCP tools exposed by the server.
package tools

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// CallTimeout is the deadline applied to every tool call.
const CallTimeout = 25 * time.Second

// Deps are the shared dependencies of all tool handlers.
type Deps struct {
	GL      *gitlab.Client
	Logger  *slog.Logger
	Redact  func(string) string
	Timeout time.Duration
}

// errorText turns a handler error into the text shown to the model.
func errorText(err error) string {
	return err.Error()
}

// safe wraps a text-returning handler into an MCP tool handler. It recovers
// panics (the SDK does not), applies the call deadline, converts errors into
// isError results and redacts the final text as the very last step.
func safe[T any](d Deps, h func(ctx context.Context, in T) (string, error)) mcp.ToolHandlerFor[T, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in T) (res *mcp.CallToolResult, _ any, _ error) {
		defer func() {
			if r := recover(); r != nil {
				msg := d.redact("внутренняя ошибка: " + fmt.Sprint(r))
				if d.Logger != nil {
					d.Logger.Error("tool handler panic", "panic", msg)
				}
				res = textResult(msg, true)
			}
		}()

		timeout := d.Timeout
		if timeout <= 0 {
			timeout = CallTimeout
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		text, err := h(ctx, in)
		if err != nil {
			return textResult(d.redact(errorText(err)), true), nil, nil
		}
		return textResult(d.redact(text), false), nil, nil
	}
}

func (d Deps) redact(s string) string {
	if d.Redact == nil {
		return s
	}
	return d.Redact(s)
}

func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}
