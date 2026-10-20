(Get-Module -ListAvailable PnP.PowerShell | Sort-Object Version -Descending | Select-Object -First 1).Version.ToString()
