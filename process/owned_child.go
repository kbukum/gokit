package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

type childExit struct {
	code   *int
	forced bool
}

type childInspector func(*exec.Cmd) (childExit, bool, error)

// The original child remains unreaped until its group is released, reserving its PID/PGID against reuse.
type ownedChild struct {
	cmd           *exec.Cmd
	pid           int
	exited        chan struct{}
	observed      chan struct{}
	exit          childExit
	observeErr    error
	done          chan struct{}
	waitErr       error
	waitStarted   bool
	groupReleased bool
	readersDone   <-chan struct{}
	closeReaders  func() error
	inspect       childInspector
	inspectMu     sync.Mutex
}

func newOwnedChild(cmd *exec.Cmd) *ownedChild {
	return newOwnedChildWithInspector(cmd, inspectChildExit)
}

func newOwnedChildWithInspector(cmd *exec.Cmd, inspect childInspector) *ownedChild {
	child := &ownedChild{
		cmd: cmd, pid: cmd.Process.Pid,
		exited: make(chan struct{}), done: make(chan struct{}),
		observed: make(chan struct{}),
		inspect:  inspect,
	}
	exited, err := child.retryObservation()
	if err != nil || exited {
		return child
	}
	go func() {
		poll := time.NewTicker(5 * time.Millisecond)
		defer poll.Stop()
		for {
			exited, err := child.retryObservation()
			if err != nil || exited {
				return
			}
			<-poll.C
		}
	}()
	return child
}

func (c *ownedChild) observationError() error {
	select {
	case <-c.observed:
		if c.observeErr != nil {
			return goerrors.Internal(fmt.Errorf("owned exit observation: %w", c.observeErr))
		}
	default:
	}
	return nil
}

func (c *ownedChild) retryObservation() (bool, error) {
	c.inspectMu.Lock()
	defer c.inspectMu.Unlock()
	select {
	case <-c.exited:
		return true, nil
	default:
	}
	state, exited, err := c.inspect(c.cmd)
	if exited && err == nil {
		c.exit = state
		close(c.exited)
	}
	if exited || err != nil {
		select {
		case <-c.observed:
		default:
			c.observeErr = err
			close(c.observed)
		}
	}
	return exited, err
}

func (c *ownedChild) startWait(ctx context.Context) {
	c.groupReleased = true
	if c.waitStarted {
		return
	}
	c.waitStarted = true
	go func() {
		if c.readersDone != nil {
			select {
			case <-c.readersDone:
			case <-ctx.Done():
				c.waitErr = errors.Join(ctx.Err(), c.closeReaders())
				<-c.readersDone
			}
		}
		c.waitErr = errors.Join(c.waitErr, c.cmd.Wait())
		close(c.done)
	}()
}

func waitOwnedExit(ctx context.Context, child *ownedChild, group bool) error {
	poll := time.NewTicker(5 * time.Millisecond)
	defer poll.Stop()
	exited := child.exited
	observation := child.observed
	observed := false
	failures := 0
	for {
		if !observed {
			confirmed, err := child.retryObservation()
			if err != nil {
				failures++
				if failures == 8 {
					return fmt.Errorf("owned exit inspection retry limit: %w", err)
				}
			}
			observed = confirmed
		}
		if observed {
			if !group {
				return nil
			}
			descendants, err := groupHasDescendants(ctx, child.pid)
			if err != nil {
				return err
			}
			if !descendants {
				return nil
			}
		}
		select {
		case <-exited:
			observed = true
			exited = nil
		case <-observation:
			observation = nil
		case <-poll.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func waitOwnedReap(ctx context.Context, child *ownedChild) error {
	select {
	case <-child.done:
		return nil
	default:
	}
	select {
	case <-child.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
