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

func TestRunScriptRiskDetection(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	ctx := context.Background()
	if r, err := (Exec{}).Run(ctx, VersionScript, nil, time.Minute); err != nil || r.ExitCode != 0 {
		t.Skip("PnP.PowerShell not installed")
	}
	risky := []string{
		"Remove-PnPList -Identity x",
		"Get-PnPList | ForEach-Object { Remove-PnPList $_ }",
		"Clear-PnPRecycleBinItem -All",
		"Invoke-PnPSPRestMethod -Url /x -Method Delete",
		"Invoke-Expression 'Get-Date'",
		"iex 'Get-Date'",
		"Invoke-WebRequest http://example.com",
		"& $cmd",
		"$item.DeleteObject()",
	}
	safe := []string{"Get-Date", "'x' | ConvertTo-Json", "Get-PnPList -Identity x -ErrorAction SilentlyContinue"}

	for _, expr := range risky {
		r, err := (Exec{}).Run(ctx, RunScript, map[string]string{EnvExpression: expr}, time.Minute)
		if err != nil || r.ExitCode != ExitConfirmationRequired || !strings.Contains(r.Stdout, "confirmationRequired") {
			t.Errorf("%q should require confirmation: exit=%d err=%v out=%q", expr, r.ExitCode, err, r.Stdout)
		}
	}
	for _, expr := range safe {
		r, _ := (Exec{}).Run(ctx, RunScript, map[string]string{EnvExpression: expr}, time.Minute)
		if r.ExitCode == ExitConfirmationRequired {
			t.Errorf("%q should not require confirmation: %q", expr, r.Stdout)
		}
	}
	r, _ := (Exec{}).Run(ctx, RunScript, map[string]string{EnvExpression: "Remove-PnPList -Identity x", EnvConfirmed: "1"}, time.Minute)
	if r.ExitCode == ExitConfirmationRequired {
		t.Error("confirmed expression must pass the guard")
	}
}

func TestCappedBuffer(t *testing.T) {
	var b cappedBuffer
	chunk := make([]byte, MaxCapture/2+1)
	for i := range 3 {
		if n, err := b.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("write %d: n=%d err=%v", i, n, err)
		}
	}
	if b.Len() != MaxCapture || !b.truncated {
		t.Errorf("len=%d truncated=%v", b.Len(), b.truncated)
	}
}

func TestRunScriptWarningsGoToStderr(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	ctx := context.Background()
	if r, err := (Exec{}).Run(ctx, VersionScript, nil, time.Minute); err != nil || r.ExitCode != 0 {
		t.Skip("PnP.PowerShell not installed")
	}
	r, err := (Exec{}).Run(ctx, RunScript, map[string]string{EnvExpression: "Write-Warning 'careful'; 'result'"}, time.Minute)
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("exit=%d err=%v stderr=%q", r.ExitCode, err, r.Stderr)
	}
	if strings.TrimSpace(r.Stdout) != "result" || !strings.Contains(r.Stderr, "WARNING: careful") {
		t.Errorf("stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
}
