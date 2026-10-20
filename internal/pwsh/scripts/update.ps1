$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
if (Get-Command Install-PSResource -ErrorAction SilentlyContinue) { Install-PSResource PnP.PowerShell -Scope CurrentUser -TrustRepository -Reinstall:$false }
else { Install-Module PnP.PowerShell -Scope CurrentUser -Force }
