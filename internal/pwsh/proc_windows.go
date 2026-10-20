//go:build windows

package pwsh

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// configure makes cancellation kill pwsh and its whole process tree.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	cmd.WaitDelay = 3 * time.Second
}
