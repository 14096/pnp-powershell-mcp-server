# Troubleshooting

## The MCP client cannot start the server

- Verify the configured command uses the absolute path to the executable and that it is executable.
- Check that `pwsh` is PowerShell 7 or later and is available in the server process's `PATH`.
- Install PnP.PowerShell in that PowerShell environment, not only in Windows PowerShell 5.1.
- Restart/reload the MCP client after changing its configuration.
- Run `pnp-powershell-mcp-server version` and use the [MCP Inspector](./client-setup.md#check-that-the-server-starts) to separate startup problems from client configuration problems.

## Not connected, expired token, or access denied

Run `pnpTestConnection` to check access: it reports the site title and URL on success, or an actionable error otherwise. Then sign in with `Connect-PnPOnline` using the intended site and app registration. If using persisted login, confirm that it was created with `-PersistLogin`, that the client ID matches, and that the token has not expired. Access denied usually means the app or signed-in identity lacks a required permission; review the requested operation and grant only the needed permission.

## Unknown cmdlet or parameter

Use `pnpSearchCmdlets` and `pnpGetCmdletDocs` to check the installed module's available names and syntax. The local cmdlet catalog is rebuilt when the module version changes. `pnpRefreshCmdlets` rebuilds it on demand; `update: true` additionally installs a newer module and requires confirmation.

## Missing or stale documentation

Cmdlet docs are fetched from the PnP.PowerShell development documentation and cached for 24 hours by module version. Cached docs can be served offline, so they may not match the currently installed module exactly. Check local `Get-Help` or the official documentation if syntax differs.

## Output was truncated

Use the returned `offset` to continue, or preferably narrow the cmdlet query with options such as `-First`, `-Identity`, or `-Query`, and shape the result with `Select-Object`. The expression runs again on each page, so paging a changing dataset may produce inconsistent results.

## Logs and caches

Server logs are written to **stderr**; stdout is reserved for the MCP protocol. The cmdlet catalog is stored under the user's cache directory at `pnp-powershell-mcp-server/catalog.json`. Documentation is cached under `pnp-powershell-mcp-server/docs/<module-version>/`. Cache files are owner-only. Removing a cache file forces the relevant data to be rebuilt or fetched again.
