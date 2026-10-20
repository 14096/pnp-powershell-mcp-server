package server

import (
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/catalog"
	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

const instructions = "Run PnP.PowerShell cmdlets. Authentication is NOT managed: call Connect-PnPOnline beforehand. " +
	"Workflow: pnpSearchCmdlets -> pnpGetCmdletDocs -> pnpRunCmdlet. Always check docs before using a cmdlet; " +
	"prefer ConvertTo-Json output; confirm before destructive Remove-* operations."

type Server struct {
	runner pwsh.Runner
	store  *catalog.Store
	docs   *docs.Client

	mu        sync.Mutex
	consulted map[string]bool
}

func New(runner pwsh.Runner, store *catalog.Store, docsClient *docs.Client) *Server {
	return &Server{runner: runner, store: store, docs: docsClient, consulted: map[string]bool{}}
}

func (s *Server) MCP(version string) *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "pnp-powershell-mcp-server", Version: version},
		&mcp.ServerOptions{Instructions: instructions},
	)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpRunCmdlet",
		Description: "Run a PnP.PowerShell expression in a fresh pwsh process ($ErrorActionPreference=Stop). Cmdlet names and parameters are validated against the installed module first. Use ConvertTo-Json for structured output. Requires prior Connect-PnPOnline (token cache).",
	}, s.runCmdlet)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpGetCmdletDocs",
		Description: "Get the up-to-date documentation (syntax, parameters, examples) for a PnP.PowerShell cmdlet. Call this before using a cmdlet.",
	}, s.getCmdletDocs)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpSearchCmdlets",
		Description: "Search the installed PnP.PowerShell cmdlets by keyword.",
	}, s.searchCmdlets)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpRefreshCmdlets",
		Description: "Rebuild the cmdlet catalog from the installed module, optionally installing the latest version from PSGallery first.",
	}, s.refreshCmdlets)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "pnpGetModuleInfo",
		Description: "Show the installed PnP.PowerShell version and catalog status.",
	}, s.moduleInfo)
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
