$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
Import-Module PnP.PowerShell
$m = Get-Module PnP.PowerShell
$cmds = Get-Command -Module PnP.PowerShell | ForEach-Object {
  $c = $_
  [pscustomobject]@{
    name = $c.Name
    parameters = @($c.Parameters.Values | ForEach-Object {
      [pscustomobject]@{ name=$_.Name; aliases=@($_.Aliases); type=$_.ParameterType.Name
        mandatory=@($_.ParameterSets.Values | Where-Object {$_.IsMandatory}).Count -gt 0 }
    })
    parameterSets = @($c.ParameterSets | ForEach-Object { $_.Name })
  }
}
[pscustomobject]@{ version=$m.Version.ToString(); helpPath=(Join-Path $m.ModuleBase 'PnP.PowerShell.dll-Help.xml'); commands=@($cmds) } | ConvertTo-Json -Depth 6 -Compress
