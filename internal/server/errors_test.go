package server

import (
	"strings"
	"testing"
)

func TestPrettyError(t *testing.T) {
	cases := []struct{ name, stderr, want, notWant string }{
		{"not signed in", "Error ID: System.InvalidOperationException,PnP.PowerShell.Commands.Lists.GetList\nDetails: You are not signed in. Please use Connect-PnPOnline to connect.", "Not connected", "Error ID"},
		{"token expired", "Error ID: X\nDetails: AADSTS70043: The refresh token has expired", "expired", ""},
		{"access denied", "Error ID: X\nDetails: Access denied. You do not have permission", "Access denied", ""},
		{"network", "Error ID: X\nDetails: No such host is known", "Network problem", ""},
		{"unknown param", "Error ID: [UNKNOWN_PARAMETER] '-Bogus'\nDetails: [UNKNOWN_PARAMETER] '-Bogus' is not valid", "[UNKNOWN_PARAMETER]", "Error ID"},
		{"syntax", "Error ID: ParseError\nDetails: Syntax error: bad", "Syntax error", "Error ID"},
		{"generic", "Error ID: Foo\nDetails: something odd", "Error ID: Foo", ""},
	}
	for _, c := range cases {
		got := prettyError(1, c.stderr)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: %q missing %q", c.name, got, c.want)
		}
		if c.notWant != "" && strings.Contains(got, c.notWant) {
			t.Errorf("%s: %q should not contain %q", c.name, got, c.notWant)
		}
	}
}
