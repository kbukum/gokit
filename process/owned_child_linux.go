package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/kbukum/gokit/fs"
)

const childObservationSupported = true

func inspectChildExit(cmd *exec.Cmd) (childExit, bool, error) {
	data, err := fs.ReadFileLimit(fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid), 4096)
	if err != nil {
		return childExit{}, false, err
	}
	return parseChildStat(data)
}

func parseChildStat(data []byte) (childExit, bool, error) {
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return childExit{}, false, fmt.Errorf("invalid owned child stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 50 {
		return childExit{}, false, fmt.Errorf("incomplete owned child stat")
	}
	if fields[0] != "Z" {
		return childExit{}, false, nil
	}
	raw, err := strconv.ParseUint(fields[49], 10, 32)
	if err != nil {
		return childExit{}, false, fmt.Errorf("invalid owned child exit status: %w", err)
	}
	status := syscall.WaitStatus(raw)
	state := childExit{forced: status.Signaled() && status.Signal() == syscall.SIGKILL}
	if status.Exited() {
		code := status.ExitStatus()
		state.code = &code
	}
	return state, true, nil
}

func groupHasDescendants(ctx context.Context, pid int) (found bool, resultErr error) {
	directory, err := os.Open("/proc")
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		entries, readErr := directory.ReadDir(128)
		for _, entry := range entries {
			member, err := strconv.Atoi(entry.Name())
			if err != nil || member == pid {
				continue
			}
			group, err := syscall.Getpgid(member)
			if errors.Is(err, syscall.ESRCH) {
				continue
			}
			if err != nil {
				return false, err
			}
			if group == pid {
				return true, nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
	}
}
