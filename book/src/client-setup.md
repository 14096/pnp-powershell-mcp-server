# Install and connect an MCP client

The server uses **stdio**, so the client launches the server process directly. Configure the command as the absolute path to the downloaded or built binary, then restart/reload the MCP client.

The repository [README](https://github.com/14096/pnp-powershell-mcp-server/blob/main/README.md#client-setup) has current configuration examples for:

- OpenCode
- Claude Code
- Cursor
- VS Code with GitHub Copilot Chat
- GitHub Copilot CLI

Client config file locations and field names can change with client versions, so check the client's MCP documentation if an example does not match your installation. Keep stdio settings intact: do not allocate a terminal or print other output to stdout, which is reserved for MCP messages.

## Check that the server starts

Run the version command to check the binary itself:

```sh
/absolute/path/to/pnp-powershell-mcp-server version
```

For a protocol-level check without an AI client, use the [MCP Inspector](https://modelcontextprotocol.io/docs/tools/inspector):

```sh
npx @modelcontextprotocol/inspector /absolute/path/to/pnp-powershell-mcp-server
```

If the client cannot start the process, check that the configured path exists, is executable, and that PowerShell 7 and PnP.PowerShell are installed where the server can find them. See [troubleshooting](./troubleshooting.md) for more.
