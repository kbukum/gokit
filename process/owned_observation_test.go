package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisedInspectionFailureCanRecover(t *testing.T) {
	t.Parallel()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
	cmd.Env = []string{"GOKIT_OWNED_CHILD=stubborn", "GORACE=atexit_sleep_ms=0"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	ConfigureSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected child inspection failure")
	var blocked atomic.Bool
	blocked.Store(true)
	owned := newOwnedChildWithInspector(cmd, func(cmd *exec.Cmd) (childExit, bool, error) {
		if blocked.Load() {
			return childExit{}, false, failure
		}
		return inspectChildExit(cmd)
	})
	cleanupObservedFixture(t, owned)
	policy := DefaultLifecyclePolicy()
	policy.GracePeriod = 50 * time.Millisecond
	supervisor := NewSupervisor(policy)
	supervisor.add(&trackedChild{cmd: cmd, pid: cmd.Process.Pid, owned: owned})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := supervisor.Shutdown(ctx, "inspection blocked"); !errors.Is(err, failure) || supervisor.Len() != 1 {
		t.Fatalf("failed cleanup ownership: %v remaining=%d", err, supervisor.Len())
	}
	blocked.Store(false)
	report, err := supervisor.Shutdown(ctx, "inspection recovered")
	if !errors.Is(err, failure) || len(report.Children) != 1 || !report.Children[0].Reaped || supervisor.Len() != 0 {
		t.Fatalf("recovered cleanup abandoned child: %+v %v", report, err)
	}
}

func TestPersistentInspectionFailureKeepsStartupOwnership(t *testing.T) {
	t.Parallel()
	for _, permanent := range []bool{false, true} {
		name := "transient"
		if permanent {
			name = "until caller retries"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected startup inspection failure")
			var blocked atomic.Bool
			blocked.Store(true)
			var owned *ownedChild
			run, err := startPersistent(t.Context(), Command{
				Binary: binary, Args: []string{"-test.run=^TestOwnedProcessChild$"},
				EnvPolicy: EnvEmpty, Env: map[string]string{"GOKIT_OWNED_CHILD": "stubborn", "GORACE": "atexit_sleep_ms=0"},
			}, PersistentConfig{
				Readiness: ReadyOnOutput, OutputMarker: "READY", ReadinessTimeout: 2 * time.Second,
				ShutdownGracePeriod: 50 * time.Millisecond,
			}, func(cmd *exec.Cmd) *ownedChild {
				owned = newOwnedChildWithInspector(cmd, func(cmd *exec.Cmd) (childExit, bool, error) {
					if blocked.Load() {
						if !permanent {
							blocked.Store(false)
						}
						return childExit{}, false, failure
					}
					return inspectChildExit(cmd)
				})
				return owned
			})
			cleanupObservedFixture(t, owned)
			if !errors.Is(err, failure) {
				t.Fatalf("inspection failure lost: %v", err)
			}
			if kind, ok := StartErrorKind(err); !ok || kind != PersistentStartObservationFailed {
				t.Fatalf("startup failure classification: %q %t", kind, ok)
			}
			if permanent {
				if run == nil {
					t.Fatal("failed startup discarded a still-owned child")
				}
				blocked.Store(false)
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				if _, err := run.Process.Shutdown(ctx); !errors.Is(err, failure) {
					t.Fatalf("retry lost startup failure: %v", err)
				}
			}
			select {
			case <-owned.done:
			default:
				t.Fatal("failed startup abandoned a live child or waiter")
			}
			select {
			case <-owned.readersDone:
			default:
				t.Fatal("failed startup abandoned output readers")
			}
		})
	}
}

func TestCompletedOutputReadinessPrecedesExit(t *testing.T) {
	t.Parallel()
	closed := make(chan struct{})
	close(closed)
	p := &PersistentProcess{owned: &ownedChild{
		exited: closed, observed: closed, exit: childExit{code: new(int)},
	}}
	cfg := PersistentConfig{Readiness: ReadyOnOutput, OutputMarker: "READY", ReadinessTimeout: time.Second}
	for range 1000 {
		if err := p.awaitReady(t.Context(), cfg, closed, closed); err != nil {
			t.Fatalf("confirmed readiness rejected after output/exit: %v", err)
		}
	}
}

func cleanupObservedFixture(t *testing.T, child *ownedChild) {
	t.Helper()
	t.Cleanup(func() {
		if err := child.cmd.Process.Kill(); err != nil && !isExited(err) {
			t.Error(err)
		}

		if child.readersDone != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			select {
			case <-child.readersDone:
			case <-ctx.Done():
				t.Error("fixture output readers retained")
			}
		}
		if !child.waitStarted {
			_ = child.cmd.Wait()
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			select {
			case <-child.done:
			case <-ctx.Done():
				t.Error("fixture waiter retained")
			}
		}
	})
}
