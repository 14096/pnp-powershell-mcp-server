$ErrorActionPreference='Stop'
$m = Get-Module -ListAvailable PnP.PowerShell | Sort-Object Version -Descending | Select-Object -First 1
[pscustomobject]@{
  psVersion = $PSVersionTable.PSVersion.ToString()
  moduleVersion = if ($m) { $m.Version.ToString() } else { '' }
} | ConvertTo-Json -Compress
