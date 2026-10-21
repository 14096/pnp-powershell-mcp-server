package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const guideURI = "pnp://best-practices"

type promptDef struct {
	prompt *mcp.Prompt
	build  func(args map[string]string) string
}

func arg(name, desc string, required bool) *mcp.PromptArgument {
	return &mcp.PromptArgument{Name: name, Description: desc, Required: required}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func promptDefs() []promptDef {
	const how = " Use pnpSearchCmdlets and pnpGetCmdletDocs before pnpRunCmdlet, prefer ConvertTo-Json with Select-Object, " +
		"and page through large results with offset. Only read data; do not change anything."
	return []promptDef{
		{
			prompt: &mcp.Prompt{
				Name:        "pnpAuditSitePermissions",
				Title:       "Audit site permissions",
				Description: "Review who has access to a SharePoint site: groups, members, site collection admins and unique permissions.",
				Arguments:   []*mcp.PromptArgument{arg("siteUrl", "URL of the site, e.g. https://contoso.sharepoint.com/sites/hub", true)},
			},
			build: func(a map[string]string) string {
				return fmt.Sprintf("Audit the permissions of the SharePoint site %s. Connect with Connect-PnPOnline first if needed. "+
					"Report site collection admins, SharePoint groups with their members, and lists or libraries with unique permissions. "+
					"Finish with a short list of risks (external users, broad groups such as Everyone, ownerless groups).%s", a["siteUrl"], how)
			},
		},
		{
			prompt: &mcp.Prompt{
				Name:        "pnpInventoryLists",
				Title:       "Inventory lists and libraries",
				Description: "Summarise the lists and libraries of a site with item counts and last modified dates.",
				Arguments:   []*mcp.PromptArgument{arg("siteUrl", "URL of the site (optional if already connected)", false)},
			},
			build: func(a map[string]string) string {
				return fmt.Sprintf("List all lists and document libraries of %s with title, template, item count and last modified date, "+
					"hidden lists excluded. Present them as a table sorted by item count.%s", orDefault(a["siteUrl"], "the connected site"), how)
			},
		},
		{
			prompt: &mcp.Prompt{
				Name:        "pnpFindCmdlet",
				Title:       "Find the right cmdlet",
				Description: "Find the PnP.PowerShell cmdlet for a task and show a validated example.",
				Arguments:   []*mcp.PromptArgument{arg("task", "What you want to do, e.g. 'add a column to a list'", true)},
			},
			build: func(a map[string]string) string {
				return fmt.Sprintf("Find the PnP.PowerShell cmdlet(s) for this task: %s. Search the catalog, read the docs of the best match and explain "+
					"the parameters that matter. Show a ready-to-run example but do not run it unless I ask.", a["task"])
			},
		},
		{
			prompt: &mcp.Prompt{
				Name:        "pnpTeamsOverview",
				Title:       "Microsoft Teams overview",
				Description: "List Teams with their channels and owners.",
			},
			build: func(map[string]string) string {
				return "List the Microsoft Teams in the tenant with display name, visibility, owner count and channels. " +
					"Assign Get-PnPTeamsTeam to a variable before piping into Select-Object (piping it directly returns an all-null object)." + how
			},
		},
	}
}

func (s *Server) addPromptsAndResources(srv *mcp.Server) {
	if s.guide != "" {
		srv.AddResource(&mcp.Resource{
			URI:         guideURI,
			Name:        "best-practices",
			Title:       "PnP.PowerShell MCP best practices",
			Description: "Authentication, error handling, output paging, security and common cmdlet patterns.",
			MIMEType:    "text/markdown",
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: guideURI, MIMEType: "text/markdown", Text: s.guide}}}, nil
		})
	}
	for _, d := range promptDefs() {
		srv.AddPrompt(d.prompt, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			args := map[string]string{}
			if req.Params != nil {
				args = req.Params.Arguments
			}
			for _, a := range d.prompt.Arguments {
				if a.Required && strings.TrimSpace(args[a.Name]) == "" {
					return nil, fmt.Errorf("argument %q is required", a.Name)
				}
			}
			return &mcp.GetPromptResult{
				Description: d.prompt.Description,
				Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: d.build(args)}}},
			}, nil
		})
	}
}
