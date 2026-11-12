# Best practices

The repository's detailed [PnP.PowerShell MCP Server best-practices guide](https://github.com/14096/pnp-powershell-mcp-server/blob/main/best-practices.md) covers authentication examples, error handling, output formatting, common SharePoint and Teams patterns, paging, and debugging. It is also available to MCP clients as the `pnp://best-practices` resource.

Key recommendations:

- Search for a cmdlet and read its current documentation before running it.
- Prefer narrow queries and select only the fields needed; use JSON for structured results.
- Use persisted sign-in for interactive work and certificate or managed identity authentication for automation where appropriate.
- Never put credentials into expressions. Grant only the app permissions required for the task.
- Read confirmation details carefully, verify the target, and explicitly approve only intended operations.
- For long output, narrow the request first. If you page a result, remember that the expression is re-executed for each page.
