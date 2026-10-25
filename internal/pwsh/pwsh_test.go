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
		"[System.IO.File]::WriteAllText('/tmp/x','y')",
		"[System.Diagnostics.Process]::Start('id')",
		"[scriptblock]::Create('Get-Date').Invoke()",
		"$ExecutionContext.InvokeCommand.InvokeScript('Get-Date')",
		"Set-Alias zz Get-Date; zz",
		"New-Object Net.WebClient",
		"Get-Content ~/.ssh/id_rsa",
		"Get-ChildItem Env:",
		"$env:HOME",
		"Import-Module /tmp/evil.dll",
		"Export-Csv -Path /tmp/x",
		"Install-Module Foo",
		"Get-PnPList | ForEach-Object Delete",
		"Grant-PnPAzureADAppSitePermission -Permissions FullControl",
		"Add-PnPSiteCollectionAdmin -Owners a@b.c",
		"Set-PnPTenant -SharingCapability Disabled",
		"Set-PnPSite -SharingCapability Disabled",
	}
	safe := []string{
		"Get-Date", "'x' | ConvertTo-Json", "Get-PnPList -Identity x -ErrorAction SilentlyContinue",
		"Get-PnPList | Where-Object { $_.Title -like 'a*' } | Select-Object Title | ConvertTo-Json",
		"[math]::Round(1.5); [string]::Join(',', 'a','b'); 'a'.Trim()",
		"$l = [System.Collections.Generic.List[string]]::new(); $l.Add('x'); $l | Sort-Object | Out-String",
	}

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

func TestRunScriptTokenAndSiteSwitchRisk(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	ctx := context.Background()
	if r, err := (Exec{}).Run(ctx, VersionScript, nil, time.Minute); err != nil || r.ExitCode != 0 {
		t.Skip("PnP.PowerShell not installed")
	}
	const site = "https://contoso.sharepoint.com"
	cases := []struct {
		expr  string
		url   string
		risky bool
	}{
		{"Get-PnPAccessToken", "", true},
		{"Get-PnPConnection", "", true},
		{"Connect-PnPOnline -Url https://evil.sharepoint.com -Interactive", site, true},
		{"Connect-PnPOnline https://evil.sharepoint.com -Interactive", site, true},
		{"Connect-PnPOnline -Url $u -Interactive", site, true},
		{"Connect-PnPOnline -Url https://evil.sharepoint.com -Interactive", "", false},
	}
	for _, tt := range cases {
		env := map[string]string{EnvExpression: tt.expr, EnvConnectURL: tt.url}
		r, err := (Exec{}).Run(ctx, RunScript, env, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.ExitCode == ExitConfirmationRequired; got != tt.risky {
			t.Errorf("%q (url=%q): risky=%v want %v; out=%q", tt.expr, tt.url, got, tt.risky, r.Stdout)
		}
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

func TestInheritedEnv(t *testing.T) {
	environ := []string{"PATH=/bin", "HOME=/h", "LC_ALL=C", "XDG_DATA_HOME=/d", "GITHUB_TOKEN=secret", "AWS_SECRET_ACCESS_KEY=s", "AZURE_CLIENT_SECRET=s2", "NOEQUALS"}
	got := strings.Join(inheritedEnv(environ, nil), " ")
	for _, want := range []string{"PATH=/bin", "HOME=/h", "LC_ALL=C", "XDG_DATA_HOME=/d"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should be inherited: %s", want, got)
		}
	}
	for _, bad := range []string{"GITHUB_TOKEN", "AWS_SECRET", "AZURE_CLIENT_SECRET", "NOEQUALS"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q must not be inherited: %s", bad, got)
		}
	}
	if got := strings.Join(inheritedEnv(environ, []string{" azure_client_secret "}), " "); !strings.Contains(got, "AZURE_CLIENT_SECRET=s2") {
		t.Errorf("extra names should be passed through: %s", got)
	}
}
