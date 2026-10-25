package pwsh

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

var (
	//go:embed scripts/catalog.ps1
	CatalogScript string
	//go:embed scripts/update.ps1
	UpdateScript string
	//go:embed scripts/run.ps1
	RunScript string
	//go:embed scripts/help.ps1
	HelpScript string
	//go:embed scripts/version.ps1
	VersionScript string
	//go:embed scripts/check.ps1
	CheckScript string
)

const (
	EnvExpression      = "PNP_MCP_EXPR"
	EnvCmdlet          = "PNP_MCP_CMDLET"
	EnvConfirmed       = "PNP_MCP_CONFIRMED"
	EnvConnectURL      = "PNP_MCP_URL"
	EnvConnectClientID = "PNP_MCP_CLIENTID"
	EnvConnectTenant   = "PNP_MCP_TENANT"
	EnvPersisted       = "PNP_MCP_PERSISTED"
	EnvProfile         = "PNP_MCP_PROFILE"
	EnvStrict          = "PNP_MCP_STRICT"
)

const ExitConfirmationRequired = 3

type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Truncated bool
}

const MaxCapture = 16 << 20

type cappedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if room := MaxCapture - b.Len(); n > room {
		b.truncated = true
		p = p[:room]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

var ErrNotFound = errors.New("PowerShell 7+ (pwsh) was not found in PATH")

type Runner interface {
	Run(ctx context.Context, script string, env map[string]string, timeout time.Duration) (Result, error)
}

type Exec struct{ ExtraEnv []string }

var inheritedNames = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "TERM": true, "LANG": true, "LANGUAGE": true,
	"TMPDIR": true, "TEMP": true, "TMP": true, "PSMODULEPATH": true, "TZ": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	"IDENTITY_ENDPOINT": true, "IDENTITY_HEADER": true, "MSI_ENDPOINT": true, "MSI_SECRET": true,
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
	"PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMW6432": true, "COMMONPROGRAMFILES": true,
	"ALLUSERSPROFILE": true, "PUBLIC": true, "COMPUTERNAME": true, "USERNAME": true, "USERDOMAIN": true, "OS": true,
	"PATHEXT": true, "COMSPEC": true, "HOMEDRIVE": true, "HOMEPATH": true, "NUMBER_OF_PROCESSORS": true,
	"PROCESSOR_ARCHITECTURE": true,
}

var inheritedPrefixes = []string{"LC_", "XDG_", "DOTNET_", "POWERSHELL_"}

func inheritedEnv(environ, extra []string) []string {
	allowed := map[string]bool{}
	for _, n := range extra {
		if n = strings.ToUpper(strings.TrimSpace(n)); n != "" {
			allowed[n] = true
		}
	}
	var out []string
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		up := strings.ToUpper(name)
		keep := inheritedNames[up] || allowed[up]
		for _, p := range inheritedPrefixes {
			keep = keep || strings.HasPrefix(up, p)
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}

func (e Exec) Run(ctx context.Context, script string, env map[string]string, timeout time.Duration) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-NoLogo", "-EncodedCommand", encode(script))
	cmd.Env = inheritedEnv(os.Environ(), e.ExtraEnv)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	configure(cmd)

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Truncated: stdout.truncated}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return res, fmt.Errorf("PowerShell timed out after %s", timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return res, fmt.Errorf("PowerShell run was cancelled: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	case errors.Is(err, exec.ErrNotFound):
		return res, fmt.Errorf("%w: %v", ErrNotFound, err)
	case err != nil:
		return res, fmt.Errorf("cannot run pwsh: %w", err)
	}
	return res, nil
}

func encode(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(u)*2)
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	return base64.StdEncoding.EncodeToString(b)
}
