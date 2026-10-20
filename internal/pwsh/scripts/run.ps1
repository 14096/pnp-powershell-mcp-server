$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$expr = $env:PNP_MCP_EXPR
try {
  Import-Module PnP.PowerShell -ErrorAction Stop
  $errs=$null; $tokens=$null
  $ast=[System.Management.Automation.Language.Parser]::ParseInput($expr,[ref]$tokens,[ref]$errs)
  if ($errs.Count -gt 0) { throw "Syntax error: $($errs[0].Message)" }
  $common = [System.Management.Automation.PSCmdlet]::CommonParameters + [System.Management.Automation.PSCmdlet]::OptionalCommonParameters
  foreach ($c in $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.CommandAst] }, $true)) {
    $n = $c.GetCommandName()
    if (-not $n -or $n -notmatch '^[A-Za-z]+-PnP') { continue }
    $info = Get-Command $n -ErrorAction SilentlyContinue
    if (-not $info) { throw "[UNKNOWN_CMDLET] '$n' does not exist in the installed PnP.PowerShell. Use pnpSearchCmdlets to find the right name (or pnpRefreshCmdlets)." }
    foreach ($e in $c.CommandElements) {
      if ($e -is [System.Management.Automation.Language.CommandParameterAst]) {
        $p = $e.ParameterName
        if ($common -contains $p) { continue }
        $hit = $info.Parameters.Keys | Where-Object { $_ -like "$p*" }
        $hit2 = $info.Parameters.Values | Where-Object { $_.Aliases -contains $p }
        if (-not $hit -and -not $hit2) { throw "[UNKNOWN_PARAMETER] '-$p' is not valid for $($info.Name). Call pnpGetCmdletDocs for '$($info.Name)' to check the syntax." }
      }
    }
  }
  $r = Invoke-Expression $expr
  if ($null -ne $r) { ($r | Out-String -Width 4096).TrimEnd() }
} catch {
  [Console]::Error.WriteLine("Error ID: " + $_.FullyQualifiedErrorId)
  [Console]::Error.WriteLine("Details: " + $_.Exception.Message)
  exit 1
}
