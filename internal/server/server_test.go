package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

type fakeRunner struct {
	res    pwsh.Result
	err    error
	script string
	env    map[string]string
	fn     func(env map[string]string) pwsh.Result
}

func (f *fakeRunner) Run(_ context.Context, script string, env map[string]string, _ time.Duration) (pwsh.Result, error) {
	f.script, f.env = script, env
	if f.fn != nil {
		return f.fn(env), f.err
	}
	return f.res, f.err
}

const catalogJSON = `{"version":"1.2.3","helpPath":"","commands":[
 {"name":"Add-PnPListItem","parameters":[],"parameterSets":[]},
 {"name":"Get-PnPList","parameters":[],"parameterSets":[]},
 {"name":"Get-PnPTeamsChannel","parameters":[],"parameterSets":[]}]}`

func newTestServer(t *testing.T, docsURL string, runner *fakeRunner) *Server {
	t.Helper()
	store := catalog.NewStore(&fakeRunner{res: pwsh.Result{Stdout: catalogJSON}}, filepath.Join(t.TempDir(), "c.json"))
	if err := store.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(runner, store, &docs.Client{BaseURL: docsURL + "/", HTTP: http.DefaultClient, CacheDir: t.TempDir(), TTL: time.Hour})
}

func text(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	return r.Content[0].(*mcp.TextContent).Text
}

func TestCmdletNameRegex(t *testing.T) {
	for _, s := range []string{"Get-PnPList", "get-pnplist", "Connect-PnPOnline"} {
		if !cmdletNameRe.MatchString(s) {
			t.Errorf("%q should match", s)
		}
	}
	for _, s := range []string{"Get-Process", "Get-PnPList; rm -rf /", "../Get-PnPList", "Get-PnP", ""} {
		if cmdletNameRe.MatchString(s) {
			t.Errorf("%q should not match", s)
		}
	}
}

func TestGetCmdletDocs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("DOC BODY")) }))
	defer ts.Close()
	s := newTestServer(t, ts.URL, &fakeRunner{})
	ctx := context.Background()

	r, _, _ := s.getCmdletDocs(ctx, nil, docsArgs{Cmdlet: "Get-Process"})
	if !r.IsError {
		t.Error("invalid name should error")
	}
	r, _, _ = s.getCmdletDocs(ctx, nil, docsArgs{Cmdlet: "Get-PnPNope"})
	if !r.IsError || !strings.Contains(text(t, r), "does not exist") {
		t.Errorf("unknown cmdlet: %s", text(t, r))
	}
	r, _, _ = s.getCmdletDocs(ctx, nil, docsArgs{Cmdlet: "get-pnplist"})
	if r.IsError || !strings.Contains(text(t, r), "DOC BODY") || !strings.Contains(text(t, r), "1.2.3") {
		t.Fatalf("unexpected: %s", text(t, r))
	}
	if got := s.undocumented("Get-PnPList"); len(got) != 0 {
		t.Errorf("cmdlet should be marked consulted, still undocumented: %v", got)
	}
}

func TestGetCmdletDocsFallsBackToLocalHelp(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	runner := &fakeRunner{res: pwsh.Result{Stdout: "LOCAL HELP"}}
	s := newTestServer(t, ts.URL, runner)

	r, _, _ := s.getCmdletDocs(context.Background(), nil, docsArgs{Cmdlet: "Get-PnPList"})
	if r.IsError || !strings.Contains(text(t, r), "LOCAL HELP") {
		t.Fatalf("unexpected: %s", text(t, r))
	}
	if runner.env[pwsh.EnvCmdlet] != "Get-PnPList" {
		t.Errorf("cmdlet not passed via env: %v", runner.env)
	}
}

func TestSearchCmdlets(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{})
	r, _, _ := s.searchCmdlets(context.Background(), nil, searchArgs{Query: "teams"})
	out := text(t, r)
	if r.IsError || !strings.Contains(out, "Get-PnPTeamsChannel") || strings.Contains(out, "Get-PnPList:") {
		t.Errorf("unexpected: %s", out)
	}
}

func TestModuleInfo(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{})
	r, _, _ := s.moduleInfo(context.Background(), nil, struct{}{})
	if out := text(t, r); !strings.Contains(out, `"installedVersion": "1.2.3"`) || !strings.Contains(out, `"cmdletCount": 3`) {
		t.Errorf("unexpected: %s", out)
	}
}

func TestRunCmdlet(t *testing.T) {
	ctx := context.Background()

	t.Run("empty expression", func(t *testing.T) {
		s := newTestServer(t, "", &fakeRunner{})
		if r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "  "}); !r.IsError {
			t.Error("expected error")
		}
	})
	t.Run("success passes expression via env and notes missing docs", func(t *testing.T) {
		runner := &fakeRunner{res: pwsh.Result{Stdout: "[]\n"}}
		s := newTestServer(t, "", runner)
		r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "Get-PnPList"})
		if r.IsError || !strings.Contains(text(t, r), "Docs not consulted") {
			t.Errorf("unexpected: %s", text(t, r))
		}
		if runner.env[pwsh.EnvExpression] != "Get-PnPList" {
			t.Errorf("env = %v", runner.env)
		}
	})
	t.Run("connection error is prettified", func(t *testing.T) {
		runner := &fakeRunner{res: pwsh.Result{ExitCode: 1, Stderr: "Error ID: X\nDetails: You are not signed in. Please use Connect-PnPOnline to connect."}}
		s := newTestServer(t, "", runner)
		r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "1"})
		if !r.IsError || !strings.Contains(text(t, r), "Not connected") {
			t.Errorf("unexpected: %s", text(t, r))
		}
	})
	t.Run("auto-connect settings are passed via env", func(t *testing.T) {
		runner := &fakeRunner{res: pwsh.Result{Stdout: "[]\n"}}
		s := newTestServer(t, "", runner)
		s.conn = Connection{URL: "https://t.sharepoint.com", ClientID: "app"}
		s.runCmdlet(ctx, nil, runArgs{Expression: "Get-PnPList"})
		if runner.env[pwsh.EnvConnectURL] != "https://t.sharepoint.com" || runner.env[pwsh.EnvConnectClientID] != "app" {
			t.Errorf("env = %v", runner.env)
		}
	})
	t.Run("auto-connect failure is prettified", func(t *testing.T) {
		runner := &fakeRunner{res: pwsh.Result{ExitCode: 1, Stderr: "Error ID: X\nDetails: [AUTO_CONNECT_FAILED] login required"}}
		s := newTestServer(t, "", runner)
		r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "1"})
		if !r.IsError || !strings.Contains(text(t, r), "-PersistLogin") {
			t.Errorf("unexpected: %s", text(t, r))
		}
	})
	t.Run("runner failure", func(t *testing.T) {
		s := newTestServer(t, "", &fakeRunner{err: context.DeadlineExceeded})
		if r, _, _ := s.runCmdlet(ctx, nil, runArgs{Expression: "1"}); !r.IsError {
			t.Error("expected error")
		}
	})
}

func TestRunCmdletTruncatesLargeOutput(t *testing.T) {
	big := "[" + strings.Repeat(`{"Title":"abcdefghij"},`, 500) + `{"Title":"z"}]`
	s := newTestServer(t, "", &fakeRunner{res: pwsh.Result{Stdout: big}})

	r, _, _ := s.runCmdlet(context.Background(), nil, runArgs{Expression: "Get-Date", MaxChars: 2000})
	out := text(t, r)
	if r.IsError || len(out) > 2000+600 || !strings.Contains(out, "[TRUNCATED] Showing") || !strings.Contains(out, "of 501 items") {
		t.Errorf("unexpected (len %d): %.300s", len(out), out)
	}
	r, _, _ = s.runCmdlet(context.Background(), nil, runArgs{Expression: "Get-Date", MaxChars: 200000})
	if strings.Contains(text(t, r), "TRUNCATED") {
		t.Error("output within the limit must not be truncated")
	}
}

func TestGuardRecoversFromPanic(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{})
	boom := func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		panic("kaboom")
	}
	r, _, err := guard[struct{}, moduleOut](s, "boomTool", boom)(context.Background(), nil, struct{}{})
	if err != nil || r == nil || !r.IsError || !strings.Contains(text(t, r), "internal error in boomTool") {
		t.Fatalf("unexpected: %v %v", r, err)
	}
}

func TestExecFailureMessages(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{})
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"pwsh missing", fmt.Errorf("%w: x", pwsh.ErrNotFound), "https://learn.microsoft.com/powershell"},
		{"timeout", fmt.Errorf("PowerShell timed out after 2s"), "raise timeoutSec"},
		{"other", fmt.Errorf("weird"), "weird"},
	}
	for _, tt := range tests {
		s.runner = &fakeRunner{err: tt.err}
		r, _, _ := s.runCmdlet(context.Background(), nil, runArgs{Expression: "Get-Date"})
		if !r.IsError || !strings.Contains(text(t, r), tt.want) {
			t.Errorf("%s: got %q, want %q", tt.name, text(t, r), tt.want)
		}
	}
}

func TestModuleInfoReportsProblems(t *testing.T) {
	s := newTestServer(t, "", &fakeRunner{err: fmt.Errorf("%w: x", pwsh.ErrNotFound)})
	r, _, _ := s.moduleInfo(context.Background(), nil, struct{}{})
	out := text(t, r)
	if !strings.Contains(out, `"healthy": false`) || !strings.Contains(out, "not found in PATH") || !strings.Contains(out, `"installedVersion": "1.2.3"`) {
		t.Errorf("unexpected: %s", out)
	}

	s = newTestServer(t, "", &fakeRunner{res: pwsh.Result{Stdout: `{"psVersion":"7.6.4","moduleVersion":"3.4.1"}`}})
	r, _, _ = s.moduleInfo(context.Background(), nil, struct{}{})
	if out := text(t, r); !strings.Contains(out, `"healthy": true`) || !strings.Contains(out, `"powershellVersion": "7.6.4"`) {
		t.Errorf("unexpected: %s", out)
	}
}
