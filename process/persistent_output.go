package process

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"
)

type guardedBuffer struct {
	mu      sync.Mutex
	buf     *limitedBuffer
	readErr error
}

func newGuardedBuffer(limit int) *guardedBuffer {
	return &guardedBuffer{buf: newLimitedBuffer(limit)}
}

func (g *guardedBuffer) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *guardedBuffer) snapshot() ([]byte, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]byte(nil), g.buf.Bytes()...), g.buf.Truncated()
}

func (g *guardedBuffer) recordError(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.readErr = errors.Join(g.readErr, err)
}

func (g *guardedBuffer) error() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.readErr
}

// Marker matching uses each new chunk plus its bounded trailing carry, not the entire retained output.
func (p *PersistentProcess) readInto(r io.Reader, dst *guardedBuffer, marker []byte, signalReady func(), wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	found := len(marker) == 0
	var carry []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				dst.recordError(writeErr)
				return
			}
			if !found {
				window := make([]byte, 0, len(carry)+n)
				window = append(window, carry...)
				window = append(window, buf[:n]...)
				if bytes.Contains(window, marker) {
					found = true
					carry = nil
					signalReady()
				} else {
					keep := min(len(marker)-1, len(window))
					carry = append(carry[:0], window[len(window)-keep:]...)
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				dst.recordError(err)
			}
			return
		}
	}
}

func (p *PersistentProcess) exitCode() *int {
	select {
	case <-p.waitCh:
		return exitCodeOf(p.cmd.ProcessState)
	default:
	}
	select {
	case <-p.owned.exited:
		return p.owned.exit.code
	default:
		return nil
	}
}

func (p *PersistentProcess) result() *Result {
	stdout, stdoutTrunc := p.stdout.snapshot()
	stderr, stderrTrunc := p.stderr.snapshot()
	return &Result{
		Stdout: stdout, StdoutTruncated: stdoutTrunc,
		Stderr: stderr, StderrTruncated: stderrTrunc,
		ExitCode: p.exitCode(), Duration: time.Since(p.start),
		Forced: p.owned.exit.forced,
	}
}
