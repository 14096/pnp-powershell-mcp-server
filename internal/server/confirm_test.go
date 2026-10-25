package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

const riskyJSON = `{"confirmationRequired":true,"actions":[{"command":"Remove-PnPList","reason":"destructive operation"}]}`

func guardedRunner() *fakeRunner {
	r := &fakeRunner{}
	r.fn = func(env map[string]string) pwsh.Result {
		if strings.Contains(env[pwsh.EnvExpression], "Remove-") && env[pwsh.EnvConfirmed] != "1" {
			return pwsh.Result{ExitCode: pwsh.ExitConfirmationRequired, Stdout: riskyJSON}
		}
		return pwsh.Result{Stdout: "done"}
	}
	return r
}

func retry(s *Server, expr, state string, action string, confirm bool) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		RequestState: state,
		InputResponses: mcp.InputResponseMap{
			elicitKey: &mcp.ElicitResult{Action: action, Content: map[string]any{elicitKey: confirm}},
		},
	}}
}

func TestUserDecision(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{})
	expr := "Remove-PnPList -Identity x"
	good := s.sign(expr)
	tests := []struct {
		name string
		req  *mcp.CallToolRequest
		want decision
	}{
		{"no request", nil, noAnswer},
		{"approved", retry(s, expr, good, "accept", true), approved},
		{"accepted but unchecked", retry(s, expr, good, "accept", false), declined},
		{"declined", retry(s, expr, good, "decline", false), declined},
		{"cancelled", retry(s, expr, good, "cancel", false), declined},
		{"forged state", retry(s, expr, "forged", "accept", true), noAnswer},
		{"state for another expression", retry(s, expr, s.sign("Remove-PnPList -Identity OTHER"), "accept", true), noAnswer},
	}
	for _, tt := range tests {
		if got := s.userDecision(tt.req, expr); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDestructiveApproved(t *testing.T) {
	runner := guardedRunner()
	s := newTestServer(t, "", runner)
	expr := "Remove-PnPList -Identity x"
	r, _, _ := s.runCmdlet(context.Background(), retry(s, expr, s.sign(expr), "accept", true), runArgs{Expression: expr})
	if r.IsError || !strings.Contains(text(t, r), "done") || runner.env[pwsh.EnvConfirmed] != "1" {
		t.Fatalf("unexpected: %s env=%v", text(t, r), runner.env)
	}
}

func TestDestructiveDeclined(t *testing.T) {
	runner := guardedRunner()
	s := newTestServer(t, "", runner)
	expr := "Remove-PnPList -Identity x"
	r, _, _ := s.runCmdlet(context.Background(), retry(s, expr, s.sign(expr), "decline", false), runArgs{Expression: expr})
	if r.IsError || !strings.Contains(text(t, r), "CANCELLED") {
		t.Fatalf("unexpected: %s", text(t, r))
	}
	if runner.env != nil {
		t.Error("declined expression must not reach PowerShell")
	}
}

func TestDestructiveTokenFlow(t *testing.T) {
	s := newTestServer(t, "", guardedRunner())
	ctx := context.Background()
	expr := "Remove-PnPList -Identity x"

	r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: expr})
	out := text(t, r)
	if !strings.Contains(out, "CONFIRMATION REQUIRED") || !strings.Contains(out, "Remove-PnPList") {
		t.Fatalf("unexpected: %s", out)
	}
	start := strings.Index(out, `confirmationToken "`) + len(`confirmationToken "`)
	token := out[start : start+strings.Index(out[start:], `"`)]

	if r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "Remove-PnPList -Identity OTHER", ConfirmationToken: token}); !strings.Contains(text(t, r), "CONFIRMATION REQUIRED") {
		t.Errorf("token must not work for another expression: %s", text(t, r))
	}
	if r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: expr, ConfirmationToken: token}); !strings.Contains(text(t, r), "CONFIRMATION REQUIRED") {
		t.Errorf("token must be single use: %s", text(t, r))
	}

	out = text(t, func() *mcp.CallToolResult { r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: expr}); return r }())
	start = strings.Index(out, `confirmationToken "`) + len(`confirmationToken "`)
	token = out[start : start+strings.Index(out[start:], `"`)]
	r, _, _ = s.runCmdlet(ctx, nil, runArgs{Expression: expr, ConfirmationToken: token})
	if r.IsError || !strings.Contains(text(t, r), "done") {
		t.Fatalf("valid token should run: %s", text(t, r))
	}
}

func TestSafeExpressionRunsDirectly(t *testing.T) {
	s := newTestServer(t, "", guardedRunner())
	r, _, _ := s.runCmdlet(context.Background(), nil, runArgs{Expression: "Get-PnPList"})
	if r.IsError || r.InputRequests != nil || !strings.Contains(text(t, r), "done") {
		t.Errorf("unexpected: %s", text(t, r))
	}
}

func TestTokensExpire(t *testing.T) {
	now := time.Now()
	tk := newTokens()
	tk.now = func() time.Time { return now }
	tok := tk.issue("expr")
	tk.now = func() time.Time { return now.Add(confirmationTTL + time.Second) }
	if tk.redeem(tok, "expr") {
		t.Error("expired token must be rejected")
	}
	if tk.redeem("bogus", "expr") {
		t.Error("unknown token must be rejected")
	}
}

func directPrompt(s *Server, elicit func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error)) {
	s.supportsElicit = func(*mcp.CallToolRequest) bool { return true }
	s.multiRoundTrip = func(*mcp.CallToolRequest) bool { return false }
	s.elicit = elicit
	s.promptTimeout = 50 * time.Millisecond
}

func TestDirectPromptApprovedAndDeclined(t *testing.T) {
	for _, tt := range []struct {
		name    string
		confirm bool
		want    string
	}{{"approved", true, "done"}, {"declined", false, "CANCELLED"}} {
		t.Run(tt.name, func(t *testing.T) {
			runner := guardedRunner()
			s := newTestServer(t, "", runner)
			directPrompt(s, func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{elicitKey: tt.confirm}}, nil
			})
			r, _, _ := s.runCmdlet(context.Background(), nil, runArgs{Expression: "Remove-PnPList -Identity x"})
			if !strings.Contains(text(t, r), tt.want) {
				t.Errorf("got %q, want %q", text(t, r), tt.want)
			}
			if ran := runner.env[pwsh.EnvConfirmed] == "1"; ran != tt.confirm {
				t.Errorf("confirmed run = %v", ran)
			}
		})
	}
}

func TestUnansweredPromptFallsBackAndStopsAsking(t *testing.T) {
	s := newTestServer(t, "", guardedRunner())
	prompts := 0
	directPrompt(s, func(ctx context.Context, _ *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		prompts++
		<-ctx.Done()
		return nil, ctx.Err()
	})

	start := time.Now()
	r, _, _ := s.runCmdlet(context.Background(), nil, runArgs{Expression: "Remove-PnPList -Identity x"})
	if !strings.Contains(text(t, r), "CONFIRMATION REQUIRED") || time.Since(start) > 5*time.Second {
		t.Fatalf("expected quick fallback, got %q after %s", text(t, r), time.Since(start))
	}
	r, _, _ = s.runCmdlet(context.Background(), nil, runArgs{Expression: "Remove-PnPList -Identity y"})
	if !strings.Contains(text(t, r), "CONFIRMATION REQUIRED") || prompts != 1 {
		t.Errorf("must not prompt again in the same session: prompts=%d", prompts)
	}
}

func TestUpdateRequiresConfirmation(t *testing.T) {
	runner := &fakeRunner{res: pwsh.Result{Stdout: catalogJSON}}
	s := newTestServer(t, "", runner)
	ctx := context.Background()

	r, out, _ := s.refreshCmdlets(ctx, nil, refreshArgs{Update: true})
	ro, _ := out.(refreshOut)
	if !ro.ConfirmationRequired || ro.ConfirmationToken == "" || !strings.Contains(text(t, r), "[CONFIRMATION REQUIRED]") {
		t.Fatalf("update must ask first: %s", text(t, r))
	}
	if runner.script == pwsh.UpdateScript {
		t.Fatal("update script ran without confirmation")
	}

	r, _, _ = s.refreshCmdlets(ctx, nil, refreshArgs{Update: true, ConfirmationToken: "bogus"})
	if !strings.Contains(text(t, r), "[CONFIRMATION REQUIRED]") {
		t.Errorf("a bad token must not authorise the update: %s", text(t, r))
	}

	runner.fn = func(map[string]string) pwsh.Result { return pwsh.Result{Stdout: catalogJSON} }
	r, _, _ = s.refreshCmdlets(ctx, nil, refreshArgs{Update: true, ConfirmationToken: ro.ConfirmationToken})
	if !strings.Contains(text(t, r), "Module update completed") {
		t.Errorf("valid token should allow the update: %s", text(t, r))
	}
	r, _, _ = s.refreshCmdlets(ctx, nil, refreshArgs{Update: true, ConfirmationToken: ro.ConfirmationToken})
	if !strings.Contains(text(t, r), "[CONFIRMATION REQUIRED]") {
		t.Error("token must be single use")
	}

	r, _, _ = s.refreshCmdlets(ctx, nil, refreshArgs{})
	if strings.Contains(text(t, r), "CONFIRMATION") {
		t.Error("a plain refresh needs no confirmation")
	}
}
