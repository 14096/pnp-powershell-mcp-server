package server

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

const instructions = "Run PnP.PowerShell cmdlets. Authentication is NOT managed: call Connect-PnPOnline beforehand. " +
	"Workflow: pnpSearchCmdlets -> pnpGetCmdletDocs -> pnpRunCmdlet. Always check docs before using a cmdlet; " +
	"prefer ConvertTo-Json output. Destructive operations (Remove-*, Clear-*, DELETE calls, ...) require user confirmation: " +
	"if pnpRunCmdlet returns [CONFIRMATION REQUIRED], show the user exactly what will happen, ask for an explicit yes/no in the conversation, " +
	"and only after a clear yes call it again with the same expression and the confirmationToken. Never obtain or reuse a token without asking the user."

type Server struct {
	runner pwsh.Runner
	store  *catalog.Store
	docs   *docs.Client

	mu        sync.Mutex
	consulted map[string]bool

	tokens *tokens
	secret []byte

	elicitation bool
	log         *slog.Logger

	noElicit       sync.Map
	elicit         func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error)
	multiRoundTrip func(*mcp.CallToolRequest) bool
	supportsElicit func(*mcp.CallToolRequest) bool
	promptTimeout  time.Duration
}

type Option func(*Server)

func WithElicitation() Option { return func(s *Server) { s.elicitation = true } }

func WithLogger(l *slog.Logger) Option { return func(s *Server) { s.log = l } }

func New(runner pwsh.Runner, store *catalog.Store, docsClient *docs.Client, opts ...Option) *Server {
	s := &Server{runner: runner, store: store, docs: docsClient, consulted: map[string]bool{}, tokens: newTokens(), secret: newSecret(), log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Server) MCP(version string) *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "pnp-powershell-mcp-server", Version: version},
		&mcp.ServerOptions{Instructions: instructions, Logger: s.log},
	)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpRunCmdlet",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(true)},
		Description: "Run a PnP.PowerShell expression in a fresh pwsh process ($ErrorActionPreference=Stop). Cmdlet names and parameters are validated against the installed module first. Use ConvertTo-Json for structured output. Requires prior Connect-PnPOnline (token cache). Destructive operations ask the user for confirmation first.",
	}, guard(s, "pnpRunCmdlet", s.runCmdlet))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpGetCmdletDocs",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "Get the up-to-date documentation (syntax, parameters, examples) for a PnP.PowerShell cmdlet. Call this before using a cmdlet.",
	}, guard(s, "pnpGetCmdletDocs", s.getCmdletDocs))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpSearchCmdlets",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "Search the installed PnP.PowerShell cmdlets by keyword.",
	}, guard(s, "pnpSearchCmdlets", s.searchCmdlets))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpRefreshCmdlets",
		Description: "Rebuild the cmdlet catalog from the installed module, optionally installing the latest version from PSGallery first.",
	}, guard(s, "pnpRefreshCmdlets", s.refreshCmdlets))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpGetModuleInfo",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		Description: "Show the installed PnP.PowerShell version and catalog status.",
	}, guard(s, "pnpGetModuleInfo", s.moduleInfo))
	return srv
}

func (s *Server) markConsulted(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consulted[strings.ToLower(name)] = true
}

func (s *Server) undocumented(expr string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, name := range cmdletUseRe.FindAllString(expr, -1) {
		key := strings.ToLower(name)
		if !s.consulted[key] && !seen[key] {
			seen[key] = true
			out = append(out, name)
		}
	}
	return out
}

func ptr[T any](v T) *T { return new(v) }

func newSecret() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func guard[In any](s *Server, name string, h func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out any, err error) {
		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("tool panicked", "tool", name, "panic", r, "stack", string(debug.Stack()))
				res, out, err = fail("internal error in %s; see the server log (run with --debug)", name)
			}
			s.log.Debug("tool call", "tool", name, "duration", time.Since(start).Round(time.Millisecond), "isError", res != nil && res.IsError)
		}()
		return h(ctx, req, in)
	}
}
