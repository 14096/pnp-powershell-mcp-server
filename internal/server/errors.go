package server

import (
	"fmt"
	"regexp"
	"strings"
)

const connectHelp = `Connect once in a PowerShell 7 session (the token cache is reused by this server):

  Connect-PnPOnline -Url "https://<tenant>.sharepoint.com" -Interactive -ClientId "<app-id>"

Other options: -DeviceLogin, -ManagedIdentity, or certificate auth (-CertificatePath).
See https://pnp.github.io/powershell/articles/authentication.html`

var (
	notConnectedRe = regexp.MustCompile(`(?i)not signed in|no connection|no current connection|Connect-PnPOnline to connect|requires a connection|connection.*(not|no).*(established|available)`)
	tokenExpiredRe = regexp.MustCompile(`(?i)AADSTS70043|AADSTS700082|AADSTS50173|AADSTS50076|AADSTS65001|token.*expired|refresh token|interaction_required|invalid_grant`)
	accessDeniedRe = regexp.MustCompile(`(?i)access denied|unauthorized|\b401\b|\b403\b|forbidden|insufficient privileges|AADSTS(65001|500011)|does not have permission`)
	networkRe      = regexp.MustCompile(`(?i)no such host|name or service not known|connection refused|timed out|unable to connect|network|SSL|socket`)
	moduleRe       = regexp.MustCompile(`(?i)module.*PnP\.PowerShell.*(not found|was not loaded)|no valid module file`)
	errIDRe        = regexp.MustCompile(`(?m)^Error ID: (.*)$`)
	detailsRe      = regexp.MustCompile(`(?ms)^Details: (.*)$`)
)

func prettyError(exitCode int, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	details := stderr
	if m := detailsRe.FindStringSubmatch(stderr); m != nil {
		details = strings.TrimSpace(m[1])
	}
	id := ""
	if m := errIDRe.FindStringSubmatch(stderr); m != nil {
		id = strings.TrimSpace(m[1])
	}

	switch {
	case strings.HasPrefix(details, "[UNKNOWN_CMDLET]"), strings.HasPrefix(details, "[UNKNOWN_PARAMETER]"), strings.HasPrefix(details, "Syntax error"):
		return "[ERROR] " + details
	case notConnectedRe.MatchString(stderr):
		return "[ERROR] Not connected to SharePoint / Microsoft 365.\n\n" + connectHelp
	case tokenExpiredRe.MatchString(stderr):
		return "[ERROR] Your sign-in has expired or needs to be renewed.\n\nRun Connect-PnPOnline again to refresh the token.\n\n" + connectHelp + "\n\nDetails: " + firstLine(details)
	case accessDeniedRe.MatchString(stderr):
		return "[ERROR] Access denied: you are connected, but lack permission for this operation.\n\n" +
			"Check that the Entra ID app / account has the permissions this cmdlet needs (see pnpGetCmdletDocs).\n\nDetails: " + firstLine(details)
	case moduleRe.MatchString(stderr):
		return "[ERROR] PnP.PowerShell is not installed or cannot be loaded.\n\nInstall it with:  Install-Module PnP.PowerShell -Scope CurrentUser"
	case networkRe.MatchString(stderr):
		return "[ERROR] Network problem while contacting Microsoft 365. Check your connection/proxy and retry.\n\nDetails: " + firstLine(details)
	}
	msg := fmt.Sprintf("[ERROR] PowerShell exited with code %d.", exitCode)
	if id != "" {
		msg += "\nError ID: " + id
	}
	if details != "" {
		msg += "\nDetails: " + details
	}
	return msg
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
