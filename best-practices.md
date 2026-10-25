# PnP.PowerShell MCP Server — Best Practices

## Prerequisites

Before using the MCP server tools, ensure the following are installed:

1. **PowerShell 7+** (`pwsh`) — available in PATH.
   - Windows: `winget install Microsoft.PowerShell`
   - macOS: `brew install --cask powershell`
   - Linux: https://docs.microsoft.com/powershell/scripting/install/installing-powershell-on-linux

2. **PnP.PowerShell module** — installed in PowerShell 7+:

   ```powershell
   Install-Module PnP.PowerShell -Scope CurrentUser
   ```

3. **A registered Entra ID application** with the permissions required for your scripts.
   See: https://pnp.github.io/powershell/articles/registerapplication.html

---

## Authentication

The MCP server does **not** manage authentication. You must call `Connect-PnPOnline`
yourself in a PowerShell session before invoking cmdlets through `pnpRunCmdlet`.

`Connect-PnPOnline` stores a token cache to disk. Each `pnpRunCmdlet` invocation
spawns a fresh `pwsh` process that reads this cache automatically, so you only need
to connect once per session (or when the token expires).

### Interactive / Device Code (development & testing)

```powershell
Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" -Interactive
# or
Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" -DeviceLogin
```

### Certificate-based (recommended for CI/CD and automation)

```powershell
Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" `
  -ClientId "<app-id>" `
  -Tenant "<tenant-id>" `
  -CertificatePath "./cert.pfx" `
  -CertificatePassword (ConvertTo-SecureString -String "<password>" -AsPlainText -Force)
```

### Managed Identity (Azure-hosted workloads)

```powershell
Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" -ManagedIdentity
```

### Check connection state before running cmdlets

```powershell
$conn = Get-PnPConnection
if ($null -eq $conn) {
    Write-Error "Not connected. Call Connect-PnPOnline first."
}
```

---

## Error Handling

The `pnpRunCmdlet` tool sets `$ErrorActionPreference = "Stop"` automatically.
This means any terminating error (including non-terminating ones promoted by
`-ErrorAction Stop`) is caught and returned as an `[ERROR]` prefixed string.

When you receive an error:

1. Read the error message carefully — it often tells you the exact problem.
2. Check that `Connect-PnPOnline` was called and the connection is active.
3. Verify you have the required permissions for the operation.
4. Use `pnpGetCmdletDocs` to review the correct parameter syntax.
5. Simplify the expression and retry with fewer parameters to isolate the issue.

**Example error response:**

```
[ERROR] PowerShell exited with code 1.
Error ID: AccessDeniedException
Details: Access denied. You do not have permission to perform this action.
```

---

## Output Formatting

By default, cmdlets return PowerShell objects that are serialised to text.
For structured output, use one of:

- **`ConvertTo-Json`** — convert output to JSON for easy parsing:
  ```powershell
  Get-PnPList | ConvertTo-Json -Depth 5
  ```
- **`Select-Object`** — limit output to specific properties:
  ```powershell
  Get-PnPList | Select-Object Title, Id, ItemCount | ConvertTo-Json
  ```
- **`Format-Table`** — human-readable table (for display, not parsing):
  ```powershell
  Get-PnPSite | Format-Table Title, Url -AutoSize
  ```

Large results are truncated (default 30,000 characters; raise `maxChars` up to
200,000). Prefer narrowing the query on the server: `-Identity`, `-Query`,
`-First`/`-PageSize`, and `Select-Object` for only the properties you need.
Valid JSON output is compacted automatically.

Avoid using `-Verbose`, `-Debug`, or `-Information` flags in automated expressions
as they add noise to the output stream.

---

## Security Guidance

- **Never embed credentials** (passwords, secrets, tokens) directly in expressions
  passed to `pnpRunCmdlet`. Use certificate-based or managed identity auth instead.
- **Prefer least-privilege** — register an Entra ID app with only the permissions
  your scripts actually need.
- **Validate inputs** — if building expressions dynamically, validate and sanitise
  any user-provided values before including them in a `pnpRunCmdlet` call.
- **Token cache location** — the PnP token cache is stored in the user's profile.
  On shared machines, ensure the profile is appropriately access-controlled.
- **Avoid destructive operations without confirmation** — cmdlets such as
  `Remove-PnPList`, `Remove-PnPSite`, etc. are irreversible. `pnpRunCmdlet`
  never runs them (or other risky operations such as `Invoke-Expression`,
  write calls via `Invoke-PnPSPRestMethod -Method Delete`, `.DeleteObject()`,
  privilege changes like `Grant-PnP*`, local file access, `$env:` reads, or any
  command outside PnP.PowerShell and the basic formatting cmdlets) without the user's explicit confirmation. If you get
  `[CONFIRMATION REQUIRED]`, show the user exactly what will happen, ask for a
  clear yes/no, and only then retry with the same expression and the returned
  `confirmationToken`. Never ask for or reuse a token without asking the user.
  Always verify the target before executing.

---

## Common Patterns

### Site Management

```powershell
# List all site collections
$sites = Get-PnPTenantSite
$sites | Select-Object Url, Title, Template, StorageUsage

# Create a new site
New-PnPSite -Type CommunicationSite -Title "Project Hub" -Url https://contoso.sharepoint.com/sites/ProjectHub

# Get site details
Get-PnPSite -Includes Owner, Usage, StorageQuota
```

### List & Library Operations

```powershell
# Get all lists
$lists = Get-PnPList
$lists | Select-Object Title, ItemCount, BaseTemplate


# Get list items with specific fields
Get-PnPListItem -List "Tasks" -Fields "Title", "Status", "AssignedTo" -PageSize 100

# Add a list item
Add-PnPListItem -List "Tasks" -Values @{"Title"="New Task"; "Status"="Not Started"}
```

### User & Permission Management

```powershell
# Get site users
$users = Get-PnPUser
$users | Select-Object Title, Email, LoginName

# Add user to group
Add-PnPGroupMember -LoginName "user@contoso.com" -Group "Site Members"

# Check permissions
Get-PnPSiteCollectionAdmin
```

### Microsoft Teams

```powershell
# Get all teams — assign first; piping Get-PnPTeamsTeam straight into
# Select-Object returns a single all-null object (see "Assign Results to a
# Variable Before Shaping Them" below)
$teams = Get-PnPTeamsTeam

$teams | Select-Object DisplayName, GroupId, Visibility

# Get team channels
Get-PnPTeamsChannel -Team "Marketing Team"
```

### Entra ID (Azure AD)

```powershell
# Get Entra ID users
$aadUsers = Get-PnPAzureADUser
$aadUsers | Select-Object DisplayName, UserPrincipalName, AccountEnabled

# Get Entra ID groups
$aadGroups = Get-PnPAzureADGroup
$aadGroups | Select-Object DisplayName, GroupTypes, SecurityEnabled

```

## Working with Complex Data

When you need to pass complex JSON or object data to a cmdlet, prefer building it as a PowerShell hashtable in the same command rather than trying to inline large JSON strings — it's less error-prone, because the expression is passed to `pwsh` as one string through an environment variable:

```powershell

$values = @{ Title = "New Task"; Status = "Not Started"; Priority = "High" }
Add-PnPListItem -List "Tasks" -Values $values
```

## Assign Results to a Variable Before Shaping Them

Some cmdlets (for example `Get-PnPTeamsTeam`) return a collection in a way that
makes `Select-Object` in the same pipeline produce a single object with null
properties. Assign first, then shape:

```powershell
$teams = Get-PnPTeamsTeam
$teams | Select-Object DisplayName, GroupId | ConvertTo-Json
```

## Paging Large Results

When output is longer than `maxChars`, `pnpRunCmdlet` cuts it and the
`[TRUNCATED]` notice names the `offset` to continue with. Call `pnpRunCmdlet`
again with the same expression and that `offset`:

- For JSON arrays, `offset` is an **item index** and pages always end at item
  boundaries, so every page is valid JSON.
- For other output, `offset` is a **character offset**.
- The expression is executed again for each page, so prefer narrowing the query
  (`-First`, `-Filter`, `Select-Object`) for expensive cmdlets, and avoid paging
  through expressions that change data.

## Finding Cmdlets

`pnpSearchCmdlets` ranks exact verb/noun matches first, then name matches, then
synopsis and parameter-name matches. Narrow with `verb` and `noun`
(`verb: "Get", noun: "List"`), and use the `related` list in the result to find
sibling cmdlets (`Set-PnPList`, `Get-PnPListItem`, ...).

## MCP Resources and Prompts

- Resource `pnp://best-practices`: this document, for clients that can attach resources.
- Prompts: `pnpAuditSitePermissions`, `pnpInventoryLists`, `pnpFindCmdlet`,
  `pnpTeamsOverview`. They are read-only starting points; clients typically
  show them as slash commands.

## Debugging and Verbose Output

```powershell
# Verbose mode
Get-PnPWeb -Verbose

# Detailed error information
Get-PnPWeb -ErrorAction Stop -ErrorVariable err; $err
```
