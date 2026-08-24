package process

import (
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
)

// startWithETXTBSYRetry starts c, retrying a transient ETXTBSY ("text file busy")
// failure with bounded exponential backoff.
//
// Executing a file that was just written races against concurrent fork/exec on other
// threads: a peer that forked while this process still held a writable descriptor to
// the target keeps that descriptor open in its child until it execs, so the kernel
// reports ETXTBSY on our exec. The window is microseconds and closes on its own, so a
// bounded backoff turns the spurious failure into a successful start. A missing or
// non-executable binary surfaces on the first attempt and is never masked.
func startWithETXTBSYRetry(c *exec.Cmd) error {
	return startRetryingETXTBSY(c.Start, time.Sleep)
}

// startRetryingETXTBSY runs start, retrying only transient ETXTBSY failures. It is split
// from startWithETXTBSYRetry so the backoff policy can be exercised with an injected
// start result and a no-op sleeper instead of a live fork/exec race.
func startRetryingETXTBSY(start func() error, sleep func(time.Duration)) error {
	const maxAttempts = 10
	const maxBackoff = 50 * time.Millisecond

	backoff := time.Millisecond
	for attempt := 1; ; attempt++ {
		err := start()
		if err == nil || attempt >= maxAttempts || !isTextFileBusy(err) {
			return err
		}
		sleep(backoff)
		backoff = min(2*backoff, maxBackoff)
	}
}

// SpawnError converts a subprocess spawn failure into a typed AppError.
//
// The OS detail is kept as the visible "<context>: <error>" message, while the error
// code is classified from the underlying failure: a missing executable becomes a
// NotFound error, a permission-denied failure becomes Forbidden, and anything else
// becomes Internal. Callers can then tell "not installed" apart from other spawn
// failures instead of seeing every failure collapse into a generic error. The original
// error is preserved as the cause so the underlying chain survives errors.Is/As.
func SpawnError(context string, err error) error {
	message := fmt.Sprintf("%s: %v", context, err)

	var appErr *goerrors.AppError
	switch {
	case stderrors.Is(err, exec.ErrNotFound) || os.IsNotExist(err):
		appErr = goerrors.NotFound("executable", "")
	case stderrors.Is(err, os.ErrPermission):
		appErr = goerrors.Forbidden("")
	default:
		appErr = goerrors.Internal(err)
	}

	appErr.Message = message
	return appErr.WithCause(err)
}
