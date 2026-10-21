package server

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

func connect(t *testing.T, s *Server, opts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := s.MCP("test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, opts).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callRun(t *testing.T, cs *mcp.ClientSession, expr string) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "pnpRunCmdlet", Arguments: map[string]any{"expression": expr}})
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func TestElicitationEndToEnd(t *testing.T) {
	for _, tt := range []struct {
		name    string
		confirm bool
		want    string
	}{
		{"user approves", true, "done"},
		{"user declines", false, "CANCELLED"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := guardedRunner()
			s := newTestServer(t, "", runner)
			s.elicitation = true
			cs := connect(t, s, &mcp.ClientOptions{
				ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					if !strings.Contains(req.Params.Message, "Remove-PnPList") {
						t.Errorf("prompt should describe the operation: %q", req.Params.Message)
					}
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": tt.confirm}}, nil
				},
			})
			if out := callRun(t, cs, "Remove-PnPList -Identity x"); !strings.Contains(out, tt.want) {
				t.Errorf("got %q, want %q", out, tt.want)
			}
			if ran := runner.env[pwsh.EnvConfirmed] == "1"; ran != tt.confirm {
				t.Errorf("confirmed run = %v, want %v", ran, tt.confirm)
			}
		})
	}
}

func TestClientWithoutElicitationGetsToken(t *testing.T) {
	s := newTestServer(t, "", guardedRunner())
	cs := connect(t, s, nil)
	if out := callRun(t, cs, "Remove-PnPList -Identity x"); !strings.Contains(out, "CONFIRMATION REQUIRED") {
		t.Errorf("got %q", out)
	}
}

func TestNativePromptIsOptIn(t *testing.T) {
	s := newTestServer(t, "", guardedRunner())
	cs := connect(t, s, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			t.Error("client must not be prompted when elicitation is not enabled")
			return &mcp.ElicitResult{Action: "cancel"}, nil
		},
	})
	if out := callRun(t, cs, "Remove-PnPList -Identity x"); !strings.Contains(out, "CONFIRMATION REQUIRED") {
		t.Errorf("got %q", out)
	}
}

func TestStructuredContent(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{res: pwsh.Result{Stdout: "[{\"Title\":\"A\"}]"}}
	cs := connect(t, newTestServer(t, "", runner), nil)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if tl.OutputSchema == nil {
			t.Errorf("%s has no output schema", tl.Name)
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "pnpRunCmdlet", Arguments: map[string]any{"expression": "Get-PnPList | ConvertTo-Json"}})
	if err != nil || res.IsError {
		t.Fatalf("%v %v", res, err)
	}
	sc, _ := res.StructuredContent.(map[string]any)
	if sc["ok"] != true {
		t.Fatalf("structured content: %v", res.StructuredContent)
	}
	if d, _ := sc["data"].([]any); len(d) != 1 {
		t.Errorf("data not parsed: %v", sc["data"])
	}

	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "pnpSearchCmdlets", Arguments: map[string]any{"query": "teams"}})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ = res.StructuredContent.(map[string]any)
	if r, _ := sc["results"].([]any); sc["ok"] != true || len(r) != 1 {
		t.Errorf("search structured content: %v", res.StructuredContent)
	}

	res, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: "pnpSearchCmdlets", Arguments: map[string]any{"query": " "}})
	if !res.IsError {
		t.Errorf("empty query should be an error")
	}
}

func TestWarningsReturned(t *testing.T) {
	runner := &fakeRunner{res: pwsh.Result{Stdout: "ok\n", Stderr: "WARNING: careful\n"}}
	s := newTestServer(t, "", runner)
	if out := callRun(t, connect(t, s, nil), "Get-Date"); !strings.Contains(out, "[WARNINGS]\nWARNING: careful") {
		t.Errorf("warnings missing: %s", out)
	}
}

func TestProgressNotifications(t *testing.T) {
	runner := &fakeRunner{res: pwsh.Result{Stdout: "ok\n"}, fn: func(map[string]string) pwsh.Result {
		time.Sleep(200 * time.Millisecond)
		return pwsh.Result{Stdout: "ok\n"}
	}}
	s := newTestServer(t, "", runner)
	s.progressInterval = 20 * time.Millisecond
	var n atomic.Int32
	cs := connect(t, s, &mcp.ClientOptions{ProgressNotificationHandler: func(context.Context, *mcp.ProgressNotificationClientRequest) { n.Add(1) }})
	params := &mcp.CallToolParams{Name: "pnpRunCmdlet", Arguments: map[string]any{"expression": "Get-Date"}}
	params.SetProgressToken("tok")
	if _, err := cs.CallTool(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	if n.Load() == 0 {
		t.Error("expected progress notifications")
	}
}
