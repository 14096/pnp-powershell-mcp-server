# Authentication

By default, the server does not manage sign-in. Your expressions connect using `Connect-PnPOnline`. Each tool call starts a new PowerShell process, so a connection object is not shared between calls; a persisted PnP.PowerShell login can be reused from the token cache.

## Interactive sign-in (default)

From PowerShell 7, persist an interactive login:

```powershell
Connect-PnPOnline `
  -Url "https://<tenant>.sharepoint.com" `
  -Interactive `
  -ClientId "<app-id>" `
  -PersistLogin
```

Then include the site URL in requests so the assistant can connect before running the relevant cmdlets. Depending on your PnP.PowerShell version and operating system, persisted-login behavior can vary. Reconnect if the token expires.

Other sign-in modes include device login, certificates, and managed identity. Choose the method that matches your environment and avoid putting secrets into assistant requests. The [best-practices guide](https://github.com/14096/pnp-powershell-mcp-server/blob/main/best-practices.md#authentication) has examples.

## Optional automatic connection

To connect to one site before every `pnpRunCmdlet` call, start the server with `--url` and `--client-id` (and optionally `--tenant`), or set `PNP_MCP_URL`, `PNP_MCP_CLIENTID`, and `PNP_MCP_TENANT`:

```sh
pnp-powershell-mcp-server \
  --url "https://<tenant>.sharepoint.com" \
  --client-id "<app-id>"
```

The URL must be HTTPS. This flow reuses a login persisted with `Connect-PnPOnline -PersistLogin`; it does not store credentials. If an expression supplies its own `Connect-PnPOnline`, the automatic connection is skipped. Connecting to a different site requires confirmation.

## Optional persisted connections for multiple tenants

Pass `--persisted-connections` (or `PNP_MCP_PERSISTED_CONNECTIONS`) a JSON array with tenant names and the app client IDs used for their persisted logins. You can provide inline JSON or prefix a file path with `@`:

```json
[
  { "tenant": "contoso", "clientId": "<app-id>" },
  { "name": "fabrikam-prod", "tenant": "fabrikam", "clientId": "<other-app-id>" }
]
```

When an expression connects to a listed tenant without specifying a client ID, the server supplies the configured client ID and interactive sign-in options. An optional `name` can be selected with the `profile` argument to `pnpRunCmdlet`. Use `--strict-connections` (or `PNP_MCP_STRICT_CONNECTIONS=1`) with this setting to reject connections to tenants that are not configured.

The configuration contains tenant names and app IDs, not secrets. Environment variables required for an authentication method are not passed to PowerShell by default; explicitly allow only the required names with `--pass-env` / `PNP_MCP_PASS_ENV`. See [security](./security.md#environment-variables) before passing secrets.
