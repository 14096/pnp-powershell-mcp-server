$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$expr = $env:PNP_MCP_EXPR
$confirmed = $env:PNP_MCP_CONFIRMED -eq '1'
$AST = 'System.Management.Automation.Language'
$destructiveVerbs = 'Remove','Clear','Reset','Revoke','Unpublish','Uninstall','Unregister','Disable','Deny','Stop','Restart','Format'
$dangerousCommands = 'Invoke-Expression','Invoke-Command','Start-Process','Invoke-WebRequest','Invoke-RestMethod','Add-Type','Set-Content','Out-File','Add-Content','Move-Item','Copy-Item','New-Item','Set-ExecutionPolicy'
$writeApiCommands = 'Invoke-PnPSPRestMethod','Invoke-PnPGraphMethod','Invoke-PnPBatch'
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

  $r = Invoke-Expression $expr
  if ($null -ne $r) { ($r | Out-String -Width 4096).TrimEnd() }
} catch {
  [Console]::Error.WriteLine("Error ID: " + $_.FullyQualifiedErrorId)
  [Console]::Error.WriteLine("Details: " + $_.Exception.Message)
  exit 1
}
