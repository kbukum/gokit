//go:build !darwin && !linux && !windows

package process

import (
	"context"
	"fmt"
	"os/exec"
)

const childObservationSupported = false

func inspectChildExit(_ *exec.Cmd) (childExit, bool, error) {
	return childExit{}, false, fmt.Errorf("owned child observation is unsupported on this platform")
}

func groupHasDescendants(_ context.Context, _ int) (bool, error) {
	return false, fmt.Errorf("owned process-group observation is unsupported on this platform")
}
