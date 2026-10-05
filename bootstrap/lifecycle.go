package bootstrap

import "context"

// beginLifecycle claims the App's single lifecycle and returns the startup context, which
// Shutdown cancels only while startup is running. The lifecycle stays active until endStartup
// or endActive releases it.
func (a *App[C]) beginLifecycle(ctx context.Context) (context.Context, error) {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.used {
		return nil, ErrLifecycleUsed
	}
	a.used, a.starting = true, true
	lifeCtx, cancel := context.WithCancelCause(ctx)
	a.cancelLifecycle, a.active, a.stopping = cancel, make(chan struct{}), make(chan struct{})
	return lifeCtx, nil
}

// endStartup marks startup finished, so a later Shutdown no longer cancels the startup context
// that components may have kept. Unless holdActive, it also ends the active lifecycle.
func (a *App[C]) endStartup(holdActive bool) {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.starting = false
	if !holdActive {
		close(a.active)
	}
}

// endActive ends an active lifecycle held past startup, letting a waiting Shutdown tear down.
func (a *App[C]) endActive() {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	close(a.active)
}

// runActiveTask runs a RunTask task and ends the active lifecycle when it returns.
func (a *App[C]) runActiveTask(ctx context.Context, task func(context.Context) error) error {
	defer a.endActive()
	return task(ctx)
}

// requestShutdown ends the App's lifecycle for new starts, cancels a running startup with
// ErrShutdownRequested, and wakes Run and RunTask. It returns the channel that closes when the
// active lifecycle ends, or nil when no lifecycle has begun.
func (a *App[C]) requestShutdown() <-chan struct{} {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.used = true
	if a.starting {
		a.cancelLifecycle(ErrShutdownRequested)
	}
	if a.stopping != nil && !isClosed(a.stopping) {
		close(a.stopping)
	}
	return a.active
}

// untilShutdown returns a context derived from ctx that Shutdown also cancels.
func (a *App[C]) untilShutdown(ctx context.Context) (context.Context, context.CancelFunc) {
	waitCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-a.stopping:
			cancel()
		case <-waitCtx.Done():
		}
	}()
	return waitCtx, cancel
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
