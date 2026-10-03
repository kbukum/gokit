package process

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

const childObservationSupported = true

func inspectChildExit(cmd *exec.Cmd) (childExit, bool, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", cmd.Process.Pid)
	if err != nil {
		return childExit{}, false, err
	}
	if int(info.Proc.P_pid) != cmd.Process.Pid {
		return childExit{}, false, fmt.Errorf("owned child identity unavailable")
	}
	const zombie = 5 // Darwin sys/proc.h: SZOMB, awaiting collection by its parent.
	if info.Proc.P_stat != zombie {
		return childExit{}, false, nil
	}
	status := syscall.WaitStatus(info.Proc.P_xstat)
	state := childExit{forced: status.Signaled() && status.Signal() == syscall.SIGKILL}
	if status.Exited() {
		code := status.ExitStatus()
		state.code = &code
	}
	return state, true, nil
}

func groupHasDescendants(ctx context.Context, pid int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	if err != nil {
		return false, err
	}
	for i := range members {
		if int(members[i].Proc.P_pid) != pid {
			return true, nil
		}
	}
	return false, nil
}
