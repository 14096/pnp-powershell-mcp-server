$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
Import-Module PnP.PowerShell
Get-Help $env:PNP_MCP_CMDLET -Full | Out-String -Width 200
