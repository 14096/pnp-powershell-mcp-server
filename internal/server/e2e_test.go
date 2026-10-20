package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

// connect wires a real MCP client to the server over in-memory transports.
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

// By default native prompts are not used, even when the client supports them,
// so clients that never display them cannot leave the call stuck.
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
