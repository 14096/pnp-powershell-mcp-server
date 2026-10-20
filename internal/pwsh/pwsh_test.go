package pwsh

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEncode(t *testing.T) {
	if got := encode("A"); got != "QQA=" {
		t.Errorf("encode(\"A\") = %q", got)
	}
}

func TestEmbeddedScripts(t *testing.T) {
	for name, s := range map[string]string{
		"catalog": CatalogScript, "update": UpdateScript, "run": RunScript,
		"help": HelpScript, "version": VersionScript,
	} {
		if s == "" {
			t.Errorf("%s script is empty", name)
		}
	}
}

func TestRunScriptValidation(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	ctx := context.Background()
	if r, err := (Exec{}).Run(ctx, VersionScript, nil, time.Minute); err != nil || r.ExitCode != 0 {
		t.Skip("PnP.PowerShell not installed")
	}
	tests := []struct {
		name, expr, wantStdout, wantStderr string
	}{
		{"unknown parameter", "Get-PnPList -Bogus 1", "", "UNKNOWN_PARAMETER"},
		{"unknown cmdlet", "Get-PnPNopeNope", "", "UNKNOWN_CMDLET"},
		{"syntax error", "1 +", "", "Syntax error"},
		{"plain expression", "'hello' | ConvertTo-Json", "hello", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := (Exec{}).Run(ctx, RunScript, map[string]string{EnvExpression: tt.expr}, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantStderr != "" && (r.ExitCode == 0 || !strings.Contains(r.Stderr, tt.wantStderr)) {
				t.Errorf("exit %d stderr %q, want %q", r.ExitCode, r.Stderr, tt.wantStderr)
			}
			if tt.wantStdout != "" && !strings.Contains(r.Stdout, tt.wantStdout) {
				t.Errorf("stdout %q, want %q", r.Stdout, tt.wantStdout)
			}
		})
	}
}
