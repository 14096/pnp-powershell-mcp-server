package server

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/14096/pnp-powershell-mcp-server/internal/docs"
	"github.com/14096/pnp-powershell-mcp-server/internal/pwsh"
)

const (
	defaultTimeout = 120 * time.Second
	maxTimeout     = 900 * time.Second
)

var (
	cmdletNameRe = regexp.MustCompile(`(?i)^[A-Za-z]+-PnP[A-Za-z0-9]+$`)
	cmdletUseRe  = regexp.MustCompile(`\b([A-Za-z]+-PnP[A-Za-z0-9]+)\b`)
)

func ok(text string) (*mcp.CallToolResult, any, error) { return result(text, false) }

func fail(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return result("[ERROR] "+fmt.Sprintf(format, args...), true)
}

func result(text string, isError bool) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isError}, nil, nil
}

type runArgs struct {
	Expression string `json:"expression" jsonschema:"PowerShell expression using PnP.PowerShell cmdlets, e.g. Get-PnPList | Select-Object Title,Id | ConvertTo-Json"`
	TimeoutSec int    `json:"timeoutSec,omitempty" jsonschema:"timeout in seconds (default 120, max 900)"`
}

func (s *Server) runCmdlet(ctx context.Context, _ *mcp.CallToolRequest, a runArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.Expression) == "" {
		return fail("expression is required")
	}
	timeout := time.Duration(a.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	timeout = min(timeout, maxTimeout)

	res, err := s.runner.Run(ctx, pwsh.RunScript, map[string]string{pwsh.EnvExpression: a.Expression}, timeout)
	if err != nil {
		return fail("%v", err)
	}
	missing := s.undocumented(a.Expression)

	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "UNKNOWN_CMDLET") {
			go func() { _ = s.store.Rebuild(context.WithoutCancel(ctx)) }()
		}
		msg := prettyError(res.ExitCode, res.Stderr)
		if len(missing) > 0 {
			msg += "\nHint: documentation was not consulted for " + strings.Join(missing, ", ") + "; call pnpGetCmdletDocs."
		}
		return result(msg, true)
	}

	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		out = "(no output)"
	}
	if len(missing) > 0 {
		out += "\n\n[NOTE] Docs not consulted this session for: " + strings.Join(missing, ", ") +
			". Verify with pnpGetCmdletDocs before relying on or repeating this."
	}
	return ok(out)
}

type docsArgs struct {
	Cmdlet string `json:"cmdlet" jsonschema:"exact cmdlet name, e.g. Get-PnPList"`
}

func (s *Server) getCmdletDocs(ctx context.Context, _ *mcp.CallToolRequest, a docsArgs) (*mcp.CallToolResult, any, error) {
	name := strings.TrimSpace(a.Cmdlet)
	if !cmdletNameRe.MatchString(name) {
		return fail("invalid cmdlet name; expected e.g. Get-PnPList")
	}

	var sb strings.Builder
	if cat := s.store.Current(); cat != nil {
		c := cat.Find(name)
		if c == nil {
			_, noun, _ := strings.Cut(name, "-")
			var similar []string
			for _, x := range cat.Search(strings.TrimPrefix(strings.ToLower(noun), "pnp"), 8) {
				similar = append(similar, x.Name)
			}
			return fail("%s does not exist in installed PnP.PowerShell %s. Similar: %s", name, cat.Version, strings.Join(similar, ", "))
		}
		name = c.Name
		fmt.Fprintf(&sb, "Module version: PnP.PowerShell %s\n", cat.Version)
	}

	md, err := s.docs.Fetch(ctx, name)
	if err == nil {
		fmt.Fprintf(&sb, "Source: online docs (%s%s)\n\n%s", docs.SiteURL, name, md)
	} else {
		r, localErr := s.runner.Run(ctx, pwsh.HelpScript, map[string]string{pwsh.EnvCmdlet: name}, 90*time.Second)
		if localErr != nil || r.ExitCode != 0 {
			return fail("could not get docs (online: %v; local: %v %s)", err, localErr, strings.TrimSpace(r.Stderr))
		}
		fmt.Fprintf(&sb, "Source: local Get-Help (online unavailable: %v)\n\n%s", err, r.Stdout)
	}
	s.markConsulted(name)
	return ok(sb.String())
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"keywords, e.g. 'list item add' or 'teams channel'"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results (default 20)"`
}

func (s *Server) searchCmdlets(_ context.Context, _ *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
	cat := s.store.Current()
	if cat == nil {
		return fail("cmdlet catalog not available; run pnpRefreshCmdlets")
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 20
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "PnP.PowerShell %s (%d cmdlets)\n", cat.Version, len(cat.Commands))
	for _, c := range cat.Search(a.Query, limit) {
		fmt.Fprintf(&sb, "- %s: %s\n", c.Name, c.Synopsis)
	}
	return ok(sb.String())
}

type refreshArgs struct {
	Update bool `json:"update,omitempty" jsonschema:"also install the latest PnP.PowerShell from PSGallery before refreshing"`
}

func (s *Server) refreshCmdlets(ctx context.Context, _ *mcp.CallToolRequest, a refreshArgs) (*mcp.CallToolResult, any, error) {
	var oldVersion string
	if c := s.store.Current(); c != nil {
		oldVersion = c.Version
	}
	var sb strings.Builder
	if a.Update {
		r, err := s.runner.Run(ctx, pwsh.UpdateScript, nil, 10*time.Minute)
		if err != nil {
			return fail("update failed: %v", err)
		}
		if r.ExitCode != 0 {
			return fail("update failed: %s", strings.TrimSpace(r.Stderr))
		}
		sb.WriteString("Module update completed.\n")
	}
	if err := s.store.Rebuild(ctx); err != nil {
		return fail("%v", err)
	}
	c := s.store.Current()
	fmt.Fprintf(&sb, "Catalog rebuilt: PnP.PowerShell %s (was %s), %d cmdlets.", c.Version, oldVersion, len(c.Commands))
	return ok(sb.String())
}

func (s *Server) moduleInfo(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	c := s.store.Current()
	if c == nil {
		return fail("catalog not loaded")
	}
	b, err := json.MarshalIndent(map[string]any{
		"installedVersion": c.Version,
		"cmdletCount":      len(c.Commands),
		"catalogUpdatedAt": c.UpdatedAt,
	}, "", "  ")
	if err != nil {
		return fail("%v", err)
	}
	return ok(string(b))
}
