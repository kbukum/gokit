package process_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	process.ConfigureSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	return cmd
}

func startSupervisedOwnedChild(t *testing.T, mode string, policy process.LifecyclePolicy) (*process.Supervisor, *exec.Cmd, string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
	cmd.Env = []string{"GOKIT_OWNED_CHILD=" + mode, "GORACE=atexit_sleep_ms=0"}
	cmd.Stderr = io.Discard
	process.ConfigureSysProcAttr(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	supervisor := process.NewSupervisor(policy)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = supervisor.Shutdown(ctx, "fallback")
	})
	supervisor.Track(cmd)
	type readiness struct {
		output string
		err    error
	}
	ready := make(chan readiness, 1)
	go func() {
		reader := bufio.NewReader(io.LimitReader(out, 66*1024))
		var output strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			output.WriteString(line)
			if readErr != nil || strings.Contains(line, "READY") {
				ready <- readiness{output: output.String(), err: readErr}
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	select {
	case result := <-ready:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return supervisor, cmd, result.output
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return nil, nil, ""
}

func TestSupervisorReportsForcedTermination(t *testing.T) {
	t.Parallel()
	supervisor, _, _ := startSupervisedOwnedChild(t, "stubborn", process.LifecyclePolicy{
		GracePeriod: 50 * time.Millisecond, IsolateProcessGroup: true, TerminateDescendants: true, KillAfterGrace: true,
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	report, err := supervisor.Shutdown(ctx, "forced")
	if err == nil || len(report.Children) != 1 || !report.Children[0].Reaped || !report.Children[0].Result.Forced {
		t.Fatalf("forced shutdown certified success: %+v %v", report, err)
	}
	again, againErr := supervisor.Shutdown(ctx, "again")
	if againErr == nil || len(again.Children) != 1 || !again.Children[0].Result.Forced {
		t.Fatalf("repeat cleanup hid escalation: %+v %v", again, againErr)
	}
}

func TestSupervisorRetryPreservesCompletedFailures(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("fixture exit handshake uses Unix inherited descriptors")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("completed child output failure")
	badReader, badWriter := io.Pipe()
	if err := badReader.CloseWithError(failure); err != nil {
		t.Fatal(err)
	}
	defer badWriter.Close()
	gateReader, gateWriter := io.Pipe()
	defer gateWriter.Close()
	exitReader, exitWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer exitReader.Close()
	t.Cleanup(func() {
		if err := exitWriter.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
	})
	supervisor := process.NewSupervisor(process.DefaultLifecyclePolicy())
	var pending process.TrackHandle
	t.Cleanup(func() {
		if err := gateReader.Close(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = supervisor.Shutdown(ctx, "fallback")
		if supervisor.Len() != 0 {
			t.Error("incomplete children retained after releasing owned output")
		}
	})
	for _, fixture := range []struct {
		mode   string
		output io.Writer
	}{
		{mode: "exit", output: badWriter},
		{mode: "stubborn", output: gateWriter},
	} {
		cmd := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
		cmd.Env = []string{"GOKIT_OWNED_CHILD=" + fixture.mode, "GORACE=atexit_sleep_ms=0"}
		cmd.Stdout = fixture.output
		cmd.Stderr = io.Discard
		if fixture.mode == "exit" {
			cmd.ExtraFiles = []*os.File{exitWriter}
		}
		process.ConfigureSysProcAttr(cmd)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		handle := supervisor.Track(cmd)
		if fixture.mode == "stubborn" {
			pending = handle
		}
	}
	if err := exitWriter.Close(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, io.LimitReader(exitReader, 1))
		exited <- err
	}()
	ready := make(chan error, 1)
	go func() {
		var first [1]byte
		_, err := gateReader.Read(first[:])
		ready <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for _, event := range []<-chan error{exited, ready} {
		select {
		case err := <-event:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	stopCtx, stopCancel := context.WithCancel(ctx)
	stopCancel()
	first, firstErr := supervisor.Shutdown(stopCtx, "partial cleanup")
	if len(first.Children) != 2 || !errors.Is(firstErr, failure) || supervisor.Len() != 1 {
		t.Fatalf("partial cleanup: %+v %v, remaining=%d", first, firstErr, supervisor.Len())
	}
	if !first.Children[0].Complete || first.Children[1].Complete {
		t.Fatal("completion did not distinguish retained output ownership")
	}
	if err := supervisor.Release(pending); err == nil {
		t.Fatal("partial cleanup ownership was discarded")
	}
	if err := gateReader.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		report, err := supervisor.Shutdown(ctx, "retry")
		if len(report.Children) != 2 || !errors.Is(err, failure) || supervisor.Len() != 0 {
			t.Fatalf("retry discarded completed failure: %+v %v", report, err)
		}
		for _, child := range report.Children {
			if !child.Reaped || !child.Complete {
				t.Fatalf("owned child not reaped: %+v", child)
			}
		}
	}
}

func TestSupervisorShutdownTerminatesTrackedChildren(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c1 := startSleeper(t)
	c2 := startSleeper(t)
	sup.Track(c1)
	sup.Track(c2)

	if sup.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", sup.Len())
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	report, err := sup.Shutdown(ctx, "test teardown")
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if len(report.Children) != 2 || !report.Children[0].Reaped || !report.Children[1].Reaped {
		t.Fatalf("incomplete shutdown report: %+v", report)
	}
	if sup.Len() != 0 {
		t.Fatalf("Len() after shutdown = %d, want 0", sup.Len())
	}

	// Both children must be reaped: their process state is set after Wait returns.
	if c1.ProcessState == nil || c2.ProcessState == nil {
		t.Fatal("expected both children reaped after Shutdown")
	}
}

func TestSupervisorShutdownIdempotent(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c := startSleeper(t)
	sup.Track(c)

	if _, err := sup.Shutdown(t.Context(), "first"); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if _, err := sup.Shutdown(t.Context(), "second"); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestSupervisorReleaseUntracks(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c := startSleeper(t)
	h := sup.Track(c)
	if sup.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", sup.Len())
	}
	if err := sup.Release(h); err != nil {
		t.Fatal(err)
	}
	if sup.Len() != 0 {
		t.Fatalf("Len() after Release = %d, want 0", sup.Len())
	}

	// Caller owns reaping now.
	_ = process.KillGroup(c)
	_ = c.Wait()
}

func TestSupervisorShutdownEmpty(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.LifecyclePolicy{})
	if _, err := sup.Shutdown(t.Context(), "noop"); err != nil {
		t.Fatalf("Shutdown on empty supervisor: %v", err)
	}
}

func TestSupervisorTrackNilAndUnstarted(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	if h := sup.Track(nil); h != 0 {
		t.Fatalf("Track(nil) = %d, want 0", h)
	}
	if h := sup.Track(exec.Command("echo")); h != 0 {
		t.Fatalf("Track(unstarted) = %d, want 0", h)
	}
	if h := sup.TrackPid(0); h != 0 {
		t.Fatalf("TrackPid(0) = %d, want 0", h)
	}
}
