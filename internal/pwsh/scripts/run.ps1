$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$expr = $env:PNP_MCP_EXPR
$confirmed = $env:PNP_MCP_CONFIRMED -eq '1'
$AST = 'System.Management.Automation.Language'
$destructiveVerbs = 'Remove','Clear','Reset','Revoke','Unpublish','Uninstall','Unregister','Disable','Deny','Stop','Restart','Format'
$dangerousCommands = 'Invoke-Expression','Invoke-Command','Start-Process','Invoke-WebRequest','Invoke-RestMethod','Add-Type','Set-Content','Out-File','Add-Content','Move-Item','Copy-Item','New-Item','Set-ExecutionPolicy'
$writeApiCommands = 'Invoke-PnPSPRestMethod','Invoke-PnPGraphMethod','Invoke-PnPBatch'
$tokenCommands = 'Get-PnPAccessToken','Get-PnPConnection'
try {
  Import-Module PnP.PowerShell -ErrorAction Stop
  $errs=$null; $tokens=$null
  $ast=[System.Management.Automation.Language.Parser]::ParseInput($expr,[ref]$tokens,[ref]$errs)
  if ($errs.Count -gt 0) { throw "Syntax error: $($errs[0].Message)" }
  $common = [System.Management.Automation.PSCmdlet]::CommonParameters + [System.Management.Automation.PSCmdlet]::OptionalCommonParameters
  $risky = [System.Collections.Generic.List[object]]::new()
  function Add-Risk($cmd, $reason) { $risky.Add([pscustomobject]@{ command = $cmd; reason = $reason }) }

  foreach ($c in $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.CommandAst] }, $true)) {
    $n = $c.GetCommandName()
    if (-not $n) { Add-Risk $c.Extent.Text 'dynamic command invocation (the command cannot be determined in advance)'; continue }
    $info = Get-Command $n -ErrorAction SilentlyContinue
    if ($info -is [System.Management.Automation.AliasInfo]) { $info = Get-Command $info.Definition -ErrorAction SilentlyContinue }
    $real = if ($info) { $info.Name } else { $n }
    if ($info -is [System.Management.Automation.ApplicationInfo] -or $info -is [System.Management.Automation.ExternalScriptInfo]) {
      Add-Risk $real 'runs an external program or script'
      continue
    }

    if ($real -match '^[A-Za-z]+-PnP') {
      if (-not $info) { throw "[UNKNOWN_CMDLET] '$n' does not exist in the installed PnP.PowerShell. Use pnpSearchCmdlets to find the right name (or pnpRefreshCmdlets)." }
      foreach ($e in $c.CommandElements) {
        if ($e -is [System.Management.Automation.Language.CommandParameterAst]) {
          $p = $e.ParameterName
          if ($common -contains $p) { continue }
          $cmdInfo = $info
          $hit = $cmdInfo.Parameters.Keys | Where-Object { $_ -like "$p*" }
          $hit2 = $cmdInfo.Parameters.Values | Where-Object { $_.Aliases -contains $p }
          if (-not $hit -and -not $hit2) { throw "[UNKNOWN_PARAMETER] '-$p' is not valid for $($cmdInfo.Name). Call pnpGetCmdletDocs for '$($cmdInfo.Name)' to check the syntax." }
        }
      }
    }

    $verb = ($real -split '-')[0]
    if ($destructiveVerbs -contains $verb) {
      Add-Risk $real 'destructive operation (removes, clears or revokes data/settings)'
    } elseif ($dangerousCommands -contains $real) {
      Add-Risk $real 'runs arbitrary code, makes web requests or writes outside SharePoint'
    } elseif ($writeApiCommands -contains $real) {
      $method = $null
      $els = @($c.CommandElements)
      for ($i = 0; $i -lt $els.Count; $i++) {
        if ($els[$i] -is [System.Management.Automation.Language.CommandParameterAst] -and 'Method' -like "$($els[$i].ParameterName)*") {
          $method = if ($els[$i].Argument) { $els[$i].Argument.Extent.Text } elseif ($i + 1 -lt $els.Count) { $els[$i + 1].Extent.Text }
        }
      }
      if ($real -eq 'Invoke-PnPBatch') { Add-Risk $real 'batch of operations (may modify or delete data)' }
      elseif ($method -and $method.Trim("'`"") -ne 'Get') { Add-Risk $real "API call with method $method (may modify or delete data)" }
    } elseif ($tokenCommands -contains $real) {
      Add-Risk $real 'exposes an access token or connection details to the conversation'
    } elseif ($real -eq 'Connect-PnPOnline' -and $env:PNP_MCP_URL) {
      $els = @($c.CommandElements); $target = $null
      for ($i = 1; $i -lt $els.Count; $i++) {
        $e = $els[$i]
        if ($e -is [System.Management.Automation.Language.CommandParameterAst] -and 'Url' -like "$($e.ParameterName)*" -and $e.ParameterName.Length -ge 1) {
          $a = if ($e.Argument) { $e.Argument } elseif ($i + 1 -lt $els.Count) { $els[$i + 1] }
          if ($a -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $target = $a.Value }
          break
        }
      }
      if (-not $target -and $els.Count -gt 1 -and $els[1] -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $target = $els[1].Value }
      if (-not $target -or $target.TrimEnd('/') -ne $env:PNP_MCP_URL.TrimEnd('/')) {
        Add-Risk $real "connects to a site other than the configured one ($($env:PNP_MCP_URL))"
      }
    }
  }
  foreach ($m in $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.InvokeMemberExpressionAst] }, $true)) {
    $name = $m.Member.Extent.Text
    if ($name -match '^(Delete|DeleteObject|Recycle|Remove|Clear)') { Add-Risk $m.Extent.Text "method call .$name() may delete data" }
  }

  if ($risky.Count -gt 0 -and -not $confirmed) {
    [pscustomobject]@{ confirmationRequired = $true; actions = @($risky) } | ConvertTo-Json -Depth 4 -Compress
    exit 3
  }

  $hasConnect = $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.CommandAst] -and $n.GetCommandName() -eq 'Connect-PnPOnline' }, $true).Count -gt 0
  if ($env:PNP_MCP_URL -and -not $hasConnect) {
    $cp = @{ Url = $env:PNP_MCP_URL; Interactive = $true }
    if ($env:PNP_MCP_CLIENTID) { $cp.ClientId = $env:PNP_MCP_CLIENTID }
    if ($env:PNP_MCP_TENANT) { $cp.Tenant = $env:PNP_MCP_TENANT }
    try { Connect-PnPOnline @cp | Out-Null }
    catch { throw "[AUTO_CONNECT_FAILED] $($_.Exception.Message)" }
  }

  $persisted = @()
  if ($env:PNP_MCP_PERSISTED) { $persisted = @($env:PNP_MCP_PERSISTED | ConvertFrom-Json) }
  if ($persisted.Count -gt 0) {
    function Get-McpTenant([string]$u) {
      if (-not $u) { return $null }
      try { $uri = [Uri]$u } catch { return $null }
      if ($uri.Scheme -ne 'https') { return $null }
      if ($uri.Host -match '^([a-z0-9-]+?)(-admin|-my)?\.sharepoint\.com$') { return $Matches[1] }
    }
    function Connect-PnPOnline {
      $interactiveAuth = 'Interactive', 'DeviceLogin'
      $otherAuth = 'ManagedIdentity', 'UserAssignedManagedIdentity', 'CertificatePath', 'CertificateBase64Encoded', 'Thumbprint', 'ClientSecret', 'Credentials', 'AccessToken', 'EnvironmentVariable', 'WebLogin', 'UseWebLogin', 'OSLogin', 'AzureADWorkloadIdentity', 'SystemAssignedManagedIdentity'
      $url = $null; $hasClient = $false; $hasInteractive = $false; $hasOther = $false
      for ($i = 0; $i -lt $args.Count; $i++) {
        $x = "$($args[$i])"
        if ($x -match '^-(\w+)(?::(.*))?$') {
          $p = $Matches[1]; $v = $Matches[2]
          if ($p -eq 'Url') { $url = if ($v) { $v } elseif ($i + 1 -lt $args.Count) { "$($args[$i + 1])" } }
          elseif ($p -in 'ClientId', 'ApplicationId') { $hasClient = $true }
          elseif ($interactiveAuth -contains $p) { $hasInteractive = $true }
          elseif ($otherAuth -contains $p) { $hasOther = $true }
        } elseif ($i -eq 0) { $url = $x }
      }
      $extra = @{}
      $tenant = Get-McpTenant $url
      if ($env:PNP_MCP_PROFILE -and -not $hasClient -and -not $hasOther) {
        $m = $persisted | Where-Object { $_.name -eq $env:PNP_MCP_PROFILE } | Select-Object -First 1
        if (-not $m) { throw "[UNKNOWN_PROFILE] '$($env:PNP_MCP_PROFILE)' is not a configured persisted connection. Configured: $(($persisted | ForEach-Object { $_.name }) -join ', ')." }
        $extra.ClientId = $m.clientId
        if (-not $hasInteractive) { $extra.Interactive = $true }
      } elseif ($tenant -and -not $hasClient -and -not $hasOther) {
        $m = $persisted | Where-Object { $_.tenant -eq $tenant } | Select-Object -First 1
        if (-not $m) {
          if ($env:PNP_MCP_STRICT -eq '1') {
            throw "[TENANT_NOT_CONFIGURED] Tenant '$tenant' has no persisted connection in the server configuration (configured: $(($persisted | ForEach-Object { $_.tenant }) -join ', ')) and strict connections are on. Add it to --persisted-connections or pass -ClientId explicitly."
          }
        }
        if ($m) {
          $extra.ClientId = $m.clientId
          if (-not $hasInteractive) { $extra.Interactive = $true }
        }
      }
      & (Get-Command Connect-PnPOnline -CommandType Cmdlet) @args @extra
    }
  }

  $r = @(Invoke-Expression $expr 3>&1 | ForEach-Object {
    if ($_ -is [System.Management.Automation.WarningRecord]) { [Console]::Error.WriteLine("WARNING: " + $_.Message); [Console]::Error.Flush() } else { $_ }
  })
  if ($r.Count -gt 0) { ($r | Out-String -Width 4096).TrimEnd() }
} catch {
  [Console]::Error.WriteLine("Error ID: " + $_.FullyQualifiedErrorId)
  [Console]::Error.WriteLine("Details: " + $_.Exception.Message)
  exit 1
}
