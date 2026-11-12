# PnP.PowerShell MCP Server

This guide covers the [PnP.PowerShell MCP Server](https://github.com/14096/pnp-powershell-mcp-server), which lets an MCP-compatible AI assistant run [PnP.PowerShell](https://pnp.github.io/powershell/) cmdlets against SharePoint and Microsoft 365.

The server communicates over MCP **stdio**. It discovers cmdlets from the installed PnP.PowerShell module, provides current cmdlet documentation, validates cmdlet and parameter names, and runs each requested expression in a fresh PowerShell process.

## Start here

- New users: [install and connect](./getting-started.md).
- Already installed: see [how to use the tools](./tools.md) and [authentication options](./authentication.md).
- Operators: review [security and confirmations](./security.md).

For detailed client-specific configuration snippets, see the repository [README](https://github.com/14096/pnp-powershell-mcp-server/blob/main/README.md). The repository also contains the [best-practices guide](https://github.com/14096/pnp-powershell-mcp-server/blob/main/best-practices.md), which is exposed to MCP clients as the `pnp://best-practices` resource.
