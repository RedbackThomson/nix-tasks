package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// killGracePeriod is how long a cancelled task's processes get to exit after
// SIGTERM before they are killed.
const killGracePeriod = 5 * time.Second

// isolateProcessGroup runs cmd in its own process group and, when its context
// is cancelled, signals the whole group. Killing only the direct child (nix)
// would leave the task's own processes running and holding the output pipes
// open, so the run would hang until they exited.
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := cmd.Process.Pid
		time.AfterFunc(killGracePeriod, func() {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		})
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	cmd.WaitDelay = killGracePeriod + time.Second
}
