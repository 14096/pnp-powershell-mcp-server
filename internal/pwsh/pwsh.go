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
)

const (
	EnvExpression = "PNP_MCP_EXPR"
	EnvCmdlet     = "PNP_MCP_CMDLET"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

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
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return res, fmt.Errorf("PowerShell timed out after %s", timeout)
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	case err != nil:
		return res, fmt.Errorf("cannot run pwsh (is PowerShell 7+ in PATH?): %w", err)
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
