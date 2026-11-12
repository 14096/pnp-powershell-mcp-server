# Security and confirmations

`pnpRunCmdlet` runs with your operating-system user's privileges and the permissions of the active PnP.PowerShell connection. It is not a sandbox. Use an Entra ID app with least-privilege permissions, keep client-side tool approval enabled, and avoid unattended use against production tenants.

## What requires confirmation

Before execution, the server parses and checks the expression. Read-only PnP operations can run without a prompt. Risky operations are held for confirmation; examples include:

- Destructive actions such as `Remove-*`, `Clear-*`, `Reset-*`, or `.Delete()`.
- Permission changes, privilege changes, app deployment, and write operations through REST or batch methods.
- Local file or environment access, external programs, arbitrary code execution, and commands outside the recognized safe PnP and formatting commands.
- Updating PnP.PowerShell with `pnpRefreshCmdlets` and `update: true`.

The complete categories and examples are maintained in the [README security section](https://github.com/14096/pnp-powershell-mcp-server/blob/main/README.md#security-and-confirmation-of-risky-operations).

## Confirmation flow

By default, the tool returns a description of the proposed operations without executing them. The assistant must show the exact actions and obtain an explicit yes/no from you. After approval, it retries the same expression with a single-use token that expires after five minutes. A declined request never reaches PowerShell.

Some MCP clients support a native confirmation dialog. Start the server with `--elicitation` to request it. Client support varies; if no response is received within 45 seconds, the server falls back to conversational confirmation. Keep the client's own tool approval enabled as an additional safeguard.

Static checks cannot catch every possible side effect. Review each requested operation and its target before approving it, especially when processing untrusted content.

## Environment variables

PowerShell receives a safe subset of the server environment by default. To expose additional variables needed for authentication, set `--pass-env NAME1,NAME2` or `PNP_MCP_PASS_ENV`. Pass only the names required; any variable exposed to PowerShell is available to expressions that run there.

## Practical safeguards

- Never include passwords, client secrets, or access tokens in expressions.
- Use least-privilege app permissions and verify the target tenant/site before write operations.
- Keep token caches and user profiles access-controlled, especially on shared machines.
- Treat confirmation as a safety net, not a substitute for review or isolation.
