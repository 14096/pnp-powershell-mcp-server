//go:build !windows

package pwsh

import (
	"os/exec"
	"syscall"
	"time"
)

// configure makes cancellation kill pwsh and everything it spawned: the child
// gets its own process group, and the whole group is killed on cancel/timeout.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 3 * time.Second // don't hang on pipes held open by stray grandchildren
}
