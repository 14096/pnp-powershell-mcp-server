# Using the tools

The server exposes these tools:

| Tool | Purpose |
| --- | --- |
| `pnpSearchCmdlets` | Search installed PnP.PowerShell cmdlets by keyword, optionally filtering by verb or noun. Results include related cmdlets. |
| `pnpGetCmdletDocs` | Fetch current documentation for a cmdlet, falling back to local `Get-Help` when online docs are unavailable. |
| `pnpRunCmdlet` | Run a PowerShell expression. PnP cmdlet and parameter names are checked before execution; risky operations require confirmation. |
| `pnpRefreshCmdlets` | Rebuild the local cmdlet catalog. Setting `update: true` installs the latest module first and requires confirmation. |
| `pnpGetModuleInfo` | Report the installed PnP.PowerShell version and catalog status. |
| `pnpTestConnection` | Connect to a site and verify access by reading its title and URL. Uses the configured `--url`, or accepts `siteUrl` with a client ID or persisted connection profile. It never returns tokens or credentials. |

When `--url` is not configured, call `pnpTestConnection` with a SharePoint `siteUrl` and either `clientId` or `profile`. The check uses interactive authentication and reads the site's web title and URL; it does not modify SharePoint data.

## Recommended workflow

Ask the assistant to **search, read docs, then run**. For example, instead of guessing syntax, have it find a suitable cmdlet with `pnpSearchCmdlets`, check it with `pnpGetCmdletDocs`, and only then call `pnpRunCmdlet`.

Expressions run in a fresh `pwsh` process and `$ErrorActionPreference` is set to `Stop`. Keep expressions focused, select only the properties needed, and convert results to JSON when structured output is useful:

```powershell
Get-PnPList | Select-Object Title, Id, ItemCount | ConvertTo-Json -Depth 3
```

Valid JSON is compacted automatically. Large responses may be truncated; the result explains how to continue with `offset`, but narrowing the query with cmdlet parameters such as `-First`, `-Query`, or `-Identity` is usually more efficient.

## Resources and prompts

- The `pnp://best-practices` resource provides the repository's [best-practices guide](https://github.com/14096/pnp-powershell-mcp-server/blob/main/best-practices.md).
- Read-only prompt starting points are `pnpAuditSitePermissions`, `pnpInventoryLists`, `pnpFindCmdlet`, and `pnpTeamsOverview`. Clients may expose prompts as slash commands.

Tool results include structured content alongside readable text. Results have an `ok` flag; for example, `pnpGetCmdletDocs` includes markdown, source, and module version, while `pnpRunCmdlet` can return output, parsed JSON data, truncation status, and confirmation details.
