package process

import (
	"context"
	"os/exec"

	"golang.org/x/sys/windows"
)

const childObservationSupported = true

func inspectChildExit(cmd *exec.Cmd) (state childExit, exited bool, resultErr error) {
	var inspectErr error
	resultErr = cmd.Process.WithHandle(func(handle uintptr) {
		status, err := windows.WaitForSingleObject(windows.Handle(handle), 0)
		if err != nil {
			inspectErr = err
			return
		}
		if status != windows.WAIT_OBJECT_0 {
			return
		}
		var code uint32
		if err := windows.GetExitCodeProcess(windows.Handle(handle), &code); err != nil {
			inspectErr = err
			return
		}
		exitCode := int(code)
		state.code, exited = &exitCode, true
	})
	if resultErr == nil {
		resultErr = inspectErr
	}
	return state, exited, resultErr
}

func groupHasDescendants(_ context.Context, _ int) (bool, error) {
	return false, nil
}
