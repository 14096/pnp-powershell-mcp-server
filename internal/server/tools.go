package server

import (
	"context"
	"encoding/json"
	"errors"
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
	Expression        string `json:"expression" jsonschema:"PowerShell expression using PnP.PowerShell cmdlets, e.g. Get-PnPList | Select-Object Title,Id | ConvertTo-Json"`
	TimeoutSec        int    `json:"timeoutSec,omitempty" jsonschema:"timeout in seconds (default 120, max 900)"`
	MaxChars          int    `json:"maxChars,omitempty" jsonschema:"max characters of output to return (default 30000, max 200000); longer output is truncated with a notice"`
	ConfirmationToken string `json:"confirmationToken,omitempty" jsonschema:"token from a previous [CONFIRMATION REQUIRED] response; only pass it after the user explicitly approved"`
}

func (s *Server) runCmdlet(ctx context.Context, req *mcp.CallToolRequest, a runArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.Expression) == "" {
		return fail("expression is required")
	}
	timeout := time.Duration(a.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	timeout = min(timeout, maxTimeout)

	answer := s.userDecision(req, a.Expression)
	if answer == declined {
		return result("[CANCELLED] The user declined. Nothing was executed.", true)
	}
	confirmed := answer == approved || (a.ConfirmationToken != "" && s.tokens.redeem(a.ConfirmationToken, a.Expression))
	s.log.Debug("running expression", "expression", a.Expression, "confirmed", confirmed, "timeout", timeout)
	res, err := s.exec(ctx, a.Expression, confirmed, timeout)
	if err != nil {
		return s.execFailure(err, timeout)
	}

	if res.ExitCode == pwsh.ExitConfirmationRequired {
		creq, perr := parseConfirmationRequest(res.Stdout)
		if perr != nil || len(creq.Actions) == 0 {
			return fail("could not interpret the confirmation request: %v", perr)
		}
		summary := describe(creq.Actions)
		if s.canElicit(req) && !s.elicitationDisabled(req) {
			if s.useMultiRoundTrip(req) {
				return &mcp.CallToolResult{
					InputRequests: mcp.InputRequestMap{elicitKey: confirmationPrompt(summary, a.Expression)},
					RequestState:  s.sign(a.Expression),
				}, nil, nil
			}
			switch s.askUser(ctx, req, confirmationPrompt(summary, a.Expression)) {
			case declined:
				return result("[CANCELLED] The user declined. Nothing was executed.", true)
			case approved:
				if res, err = s.exec(ctx, a.Expression, true, timeout); err != nil {
					return s.execFailure(err, timeout)
				}
			}
		}
		if res.ExitCode == pwsh.ExitConfirmationRequired {
			return ok(fmt.Sprintf("[CONFIRMATION REQUIRED] Nothing was executed. This expression contains risky operations:\n%s\n\n"+
				"Show this to the user and ask for an explicit yes/no in the conversation. Only if they clearly approve, call pnpRunCmdlet again "+
				"with the identical expression and confirmationToken %q (valid %d minutes, single use).",
				summary, s.tokens.issue(a.Expression), int(confirmationTTL.Minutes())))
		}
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

	out := formatOutput(res.Stdout, clampMaxChars(a.MaxChars))
	if res.Truncated {
		out += "\n\n[TRUNCATED] Output exceeded the server's capture limit and was cut. " + narrowHint
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
	var version string
	if cat, _ := s.store.Wait(ctx); cat != nil {
		version = cat.Version
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

	md, err := s.docs.Fetch(ctx, name, version)
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

func (s *Server) searchCmdlets(ctx context.Context, _ *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
	cat, err := s.store.Wait(ctx)
	if err != nil {
		return fail("%v", err)
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

func (s *Server) moduleInfo(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	diag := pwsh.Diagnose(ctx, s.runner)
	info := map[string]any{"powershellVersion": diag.PSVersion, "healthy": diag.OK()}
	if len(diag.Problems) > 0 {
		info["problems"] = diag.Problems
	}
	if c, err := s.store.Wait(ctx); err == nil {
		info["installedVersion"] = c.Version
		info["cmdletCount"] = len(c.Commands)
		info["catalogUpdatedAt"] = c.UpdatedAt
	} else {
		info["catalogError"] = err.Error()
	}
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fail("%v", err)
	}
	return ok(string(b))
}

// execFailure explains why PowerShell could not be run to completion.
func (s *Server) execFailure(err error, timeout time.Duration) (*mcp.CallToolResult, any, error) {
	s.log.Warn("PowerShell run failed", "error", err)
	switch {
	case errors.Is(err, pwsh.ErrNotFound):
		return fail("%v.\n%s", pwsh.ErrNotFound, pwsh.InstallPwshHint)
	case strings.Contains(err.Error(), "timed out"):
		return fail("The command did not finish within %s and was stopped (including any processes it started). "+
			"Narrow the query, or raise timeoutSec (max %d).", timeout, int(maxTimeout.Seconds()))
	}
	return fail("%v", err)
}

func (s *Server) exec(ctx context.Context, expr string, confirmed bool, timeout time.Duration) (pwsh.Result, error) {
	env := map[string]string{pwsh.EnvExpression: expr}
	if confirmed {
		env[pwsh.EnvConfirmed] = "1"
	}
	return s.runner.Run(ctx, pwsh.RunScript, env, timeout)
}
