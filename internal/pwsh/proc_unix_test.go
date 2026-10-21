//go:build !windows

package pwsh

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutKillsChildProcesses(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := `$p = Start-Process sleep -ArgumentList 60 -PassThru
Set-Content -Path $env:PNP_TEST_PIDFILE -Value $p.Id
Start-Sleep 60`

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if b, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(b)) != "" {
				cancel()
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	start := time.Now()
	_, err := Exec{}.Run(ctx, script, map[string]string{"PNP_TEST_PIDFILE": pidFile}, 2*time.Minute)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancel error, got %v", err)
	}
	if took := time.Since(start); took > time.Minute {
		t.Errorf("run took %s", took)
	}

	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("child never started: %v", err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("child process %d survived the timeout", pid)
}

func TestCancelStopsRun(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(time.Second); cancel() }()
	start := time.Now()
	_, err := Exec{}.Run(ctx, "Start-Sleep 60", nil, time.Minute)
	if err == nil || time.Since(start) > 15*time.Second {
		t.Errorf("cancel should stop the run quickly: err=%v after %s", err, time.Since(start))
	}
}
