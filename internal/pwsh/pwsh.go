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

type Exec struct{}

func (Exec) Run(ctx context.Context, script string, env map[string]string, timeout time.Duration) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-NoLogo", "-EncodedCommand", encode(script))
	cmd.Env = os.Environ()
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
