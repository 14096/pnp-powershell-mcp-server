# Getting started

## Requirements

- PowerShell 7 or later (`pwsh`) available in `PATH`.
- PnP.PowerShell installed in that PowerShell installation:

  ```powershell
  Install-Module PnP.PowerShell -Scope CurrentUser
  ```

- An Entra ID app registration configured for PnP.PowerShell and the permissions your work requires. See the [PnP.PowerShell registration guide](https://pnp.github.io/powershell/articles/registerapplication.html).
- An MCP-compatible client. For building from source, install Go as specified in `go.mod`.

## Install the server

Download a prebuilt binary from [GitHub Releases](https://github.com/14096/pnp-powershell-mcp-server/releases) and verify it against the release `checksums.txt`, or build/install with Go:

```sh
git clone https://github.com/14096/pnp-powershell-mcp-server.git
cd pnp-powershell-mcp-server
make build
bin/pnp-powershell-mcp-server version
```

The server binary is `bin/pnp-powershell-mcp-server` (add `.exe` on Windows). Use its **absolute path** in your MCP client configuration. Alternatively, install the latest published version with:

```sh
go install github.com/14096/pnp-powershell-mcp-server@latest
```

## Sign in and make a first request

For the default authentication flow, sign in once from PowerShell and persist the login so fresh PowerShell processes can reuse its token:

```powershell
Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" -Interactive -ClientId "<app-id>" -PersistLogin
```

Configure your MCP client to start the server, then restart or reload that client. Ask the assistant to verify access with `pnpTestConnection`, then to perform a simple read-only task, such as listing the site's lists. A typical discovery flow is:

1. Search for relevant cmdlets with `pnpSearchCmdlets`.
2. Read current syntax with `pnpGetCmdletDocs`.
3. Run a read-only expression with `pnpRunCmdlet`.

The [client setup chapter](./client-setup.md) links to ready-to-use configuration examples. The first server start builds its local cmdlet catalog in the background; tools that require it wait for the catalog to be ready.
