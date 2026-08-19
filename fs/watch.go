package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	apperrors "github.com/kbukum/gokit/errors"

	"github.com/fsnotify/fsnotify"
)

// defaultWatchBuffer is the default bounded capacity of the emitted batch channel.
// Backpressure is intentional: a slow consumer stalls batch delivery rather than
// growing an unbounded queue.
const defaultWatchBuffer = 1024

// FsWatcher is a recursive, debounced filesystem-tree watcher. Construct it with a
// trailing-edge debounce window, then call Watch with the roots to observe and a
// context. Each call is independent: it installs its own platform watcher, kept alive
// for exactly as long as the returned channel is delivering — canceling the context
// tears the OS watch down and closes the channel.
type FsWatcher struct {
	debounce time.Duration
	buffer   int
}

// NewFsWatcher creates a watcher with the given trailing-edge debounce window.
func NewFsWatcher(debounce time.Duration) *FsWatcher {
	return &FsWatcher{debounce: debounce, buffer: defaultWatchBuffer}
}

// WithBuffer overrides the bounded channel capacity, clamped to at least one.
func (w *FsWatcher) WithBuffer(buffer int) *FsWatcher {
	if buffer < 1 {
		buffer = 1
	}
	return &FsWatcher{debounce: w.debounce, buffer: buffer}
}

// Watch observes roots recursively, returning a channel of debounced FsChangeBatch
// values. Newly created subdirectories are added to the watch as they appear. If the
// platform watcher reports an error (typically a queue overflow), the next batch
// carries RescanRequested so consumers re-evaluate the tree instead of silently
// missing dropped changes. The channel is closed when ctx is canceled or the watcher
// stops. The watching goroutine is owned by this call: it is canceled through ctx,
// drains no unbounded state, and releases the OS watch on exit.
//
// It returns an INVALID_INPUT error when roots is empty, and a typed error (cause
// preserved) when a root cannot be watched.
func (w *FsWatcher) Watch(ctx context.Context, roots []string) (<-chan FsChangeBatch, error) {
	if len(roots) == 0 {
		return nil, apperrors.InvalidInput("roots",
			"filesystem watch requires at least one root path")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, watchError("create filesystem watcher", "", err)
	}
	for _, root := range roots {
		if err := addRecursive(watcher, root); err != nil {
			_ = watcher.Close()
			return nil, watchError("watch path", root, err)
		}
	}

	out := make(chan FsChangeBatch, w.buffer)
	go w.run(ctx, watcher, out)
	return out, nil
}

func (w *FsWatcher) run(ctx context.Context, watcher *fsnotify.Watcher, out chan<- FsChangeBatch) {
	defer close(out)
	defer func() { _ = watcher.Close() }()

	pending := make(map[string]struct{})
	rescan := false
	timer := time.NewTimer(w.debounce)
	timer.Stop()
	timerActive := false
	defer timer.Stop()

	arm := func() {
		if !timerActive {
			timer.Reset(w.debounce)
			timerActive = true
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			pending[event.Name] = struct{}{}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addRecursive(watcher, event.Name)
				}
			}
			arm()
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			rescan = true
			arm()
		case <-timer.C:
			timerActive = false
			batch := newFsChangeBatch(pending, rescan)
			pending = make(map[string]struct{})
			rescan = false
			select {
			case out <- batch:
			case <-ctx.Done():
				return
			}
		}
	}
}

// addRecursive watches root and, when root is a directory, every subdirectory beneath
// it, mirroring the recursive watch semantics of the reference watcher.
func addRecursive(watcher *fsnotify.Watcher, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return watcher.Add(root)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}

func watchError(action, path string, err error) error {
	message := fmt.Sprintf("failed to %s", action)
	if path != "" {
		message = fmt.Sprintf("failed to %s '%s'", action, path)
	}
	code, status := watchErrorCode(err)
	return apperrors.New(code, fmt.Sprintf("%s: %v", message, err), status).WithCause(err)
}

// watchErrorCode classifies a watch failure into a typed error code so it carries a
// meaningful status instead of a blanket internal error.
func watchErrorCode(err error) (code apperrors.ErrorCode, status int) {
	switch {
	case os.IsNotExist(err):
		return apperrors.ErrCodeNotFound, 404
	case os.IsPermission(err):
		return apperrors.ErrCodeForbidden, 403
	default:
		return osErrorCode(err)
	}
}
