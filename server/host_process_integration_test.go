//go:build integration

package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/process"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/util"
)

// These tests re-run the test binary as a real host process. The child mode is selected by
// an explicit environment variable on an otherwise empty environment.
const (
	hostModeEnv     = "GOKIT_HOST_PROCESS_MODE"
	hostLogEnv      = "GOKIT_HOST_PROCESS_LOG"
	hostReadyMarker = "host-ready "
	hostResultLine  = "host-result "

	startupBudget  = 10 * time.Second
	callBudget     = 2 * time.Second
	shutdownBudget = 5 * time.Second
	cleanupBudget  = 10 * time.Second
	outputBudget   = 1 << 20

	taskExitCode     = 3
	rollbackExitCode = 4
)

type hostAddresses struct {
	Public   string `json:"public"`
	Internal string `json:"internal"`
}

type taskOutcome struct {
	TaskError       bool   `json:"task_error"`
	TaskCause       bool   `json:"task_cause"`
	ShutdownError   bool   `json:"shutdown_error"`
	ShutdownCause   bool   `json:"shutdown_cause"`
	Message         string `json:"message"`
	BorrowedClosed  bool   `json:"borrowed_closed"`
	BorrowedCloseOK bool   `json:"borrowed_close_ok"`
}

type rollbackOutcome struct {
	Phase             string `json:"phase"`
	StartError        bool   `json:"start_error"`
	FailedComponent   string `json:"failed_component"`
	ComponentRollback string `json:"component_rollback"`
	AppRollback       string `json:"app_rollback"`
	PortReleased      bool   `json:"port_released"`
}

// TestHostProcessChild is the child entry point; it is skipped unless a mode is selected.
func TestHostProcessChild(t *testing.T) {
	mode := os.Getenv(hostModeEnv)
	if mode == "" {
		t.Skip("child process entry point")
	}
	var code int
	switch mode {
	case "service":
		code = runServiceHost()
	case "task":
		code = runFailingTask()
	case "rollback":
		code = runPartialStart()
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", mode)
		code = 2
	}
	os.Exit(code)
}

func TestHostProcessServiceReleasesOwnedResources(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "host.log")
	cfg := process.DefaultPersistentConfig()
	cfg.Readiness, cfg.OutputMarker = process.ReadyOnOutput, hostReadyMarker
	cfg.ReadinessTimeout = startupBudget
	cfg.ShutdownGracePeriod, cfg.Lifecycle.GracePeriod = shutdownBudget, shutdownBudget
	cfg.MaxCaptureBytes = outputBudget
	run, err := process.StartPersistent(t.Context(), childCommand(t, "service", logPath), cfg)
	if run != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), cleanupBudget)
			defer cancel()
			if outcome, err := run.Process.Shutdown(ctx); err != nil || !outcome.Complete {
				t.Errorf("owned host cleanup: complete=%v error=%v", outcome.Complete, err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	addrs := decodeMarked[hostAddresses](t, run.Startup.Stdout, hostReadyMarker)

	client := &http.Client{Timeout: callBudget}
	t.Cleanup(client.CloseIdleConnections)
	for _, probe := range []struct {
		addr, path string
		want       int
	}{
		{addrs.Public, "/internal", http.StatusNotFound},
		{addrs.Internal, "/internal", http.StatusOK},
		{addrs.Internal, "/slow", http.StatusNotFound},
	} {
		if got := statusOf(t, client, "http://"+probe.addr+probe.path); got != probe.want {
			t.Errorf("GET %s%s = %d, want %d", probe.addr, probe.path, got, probe.want)
		}
	}

	// Streams outlive a single call budget, so they are bounded by the shutdown budget instead.
	streamCtx, cancelStreams := context.WithTimeout(t.Context(), startupBudget+shutdownBudget)
	defer cancelStreams()
	stream := openLine(t, streamCtx, "http://"+addrs.Public+"/events")
	slow := openLine(t, streamCtx, "http://"+addrs.Public+"/slow")
	if slow.first != "accepted\n" {
		t.Fatalf("slow first line = %q", slow.first)
	}

	type shutdownResult struct {
		outcome process.ShutdownOutcome
		err     error
		elapsed time.Duration
	}
	done := make(chan shutdownResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownBudget)
		defer cancel()
		start := time.Now()
		outcome, err := run.Process.Shutdown(ctx)
		done <- shutdownResult{outcome, err, time.Since(start)}
	}()

	rest, err := io.ReadAll(io.LimitReader(slow.body, 4096))
	if err != nil || string(rest) != "completed\n" {
		t.Errorf("accepted request did not complete during shutdown: %q, %v", rest, err)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(stream.body, outputBudget)); err != nil {
		t.Errorf("open stream did not end cleanly: %v", err)
	}

	result := <-done
	if result.err != nil || !result.outcome.Complete || result.outcome.Result == nil {
		t.Fatalf("shutdown: complete=%v error=%v", result.outcome.Complete, result.err)
	}
	if code := result.outcome.Result.ExitCode; code == nil || *code != 0 || strings.Contains(string(result.outcome.Result.Stderr), "DATA RACE") {
		t.Fatalf("exit code = %v, stderr:\n%s", code, result.outcome.Result.Stderr)
	}
	if result.elapsed > shutdownBudget {
		t.Errorf("shutdown took %v, budget %v", result.elapsed, shutdownBudget)
	}
	t.Logf("service shutdown took %v", result.elapsed)
	for _, addr := range []string{addrs.Public, addrs.Internal} {
		if portOpen(addr) {
			t.Errorf("listener %s remained after shutdown", addr)
		}
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "Application shutdown complete") {
		t.Errorf("last owned log line = %q, want shutdown completion", last)
	}
	if strings.Contains(string(logged), "after-release") {
		t.Error("owned logger accepted a write after the App released it")
	}
}

func TestHostProcessTaskReportsTaskAndCleanupFailures(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "borrowed.log")
	result := runChild(t, "task", logPath, taskExitCode)
	got := decodeMarked[taskOutcome](t, result.Stdout, hostResultLine)
	if !got.TaskError || !got.TaskCause || !got.ShutdownError || !got.ShutdownCause {
		t.Errorf("task outcome not independently inspectable: %+v", got)
	}
	if !strings.Contains(got.Message, "task failed") || !strings.Contains(got.Message, "cleanup failed") {
		t.Errorf("message = %q, want both failures", got.Message)
	}
	if got.BorrowedClosed || !got.BorrowedCloseOK {
		t.Errorf("borrowed logger was released by the App: %+v", got)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "borrowed-still-open") {
		t.Error("borrowed logger did not accept writes after the App shut down")
	}
}

func TestHostProcessPartialStartRollsBack(t *testing.T) {
	result := runChild(t, "rollback", filepath.Join(t.TempDir(), "host.log"), rollbackExitCode)
	got := decodeMarked[rollbackOutcome](t, result.Stdout, hostResultLine)
	if got.Phase != string(bootstrap.PhaseStart) || !got.StartError || got.FailedComponent != "database" {
		t.Errorf("startup failure = %+v", got)
	}
	if !strings.Contains(got.ComponentRollback, "stop cache: cache stop failed") {
		t.Errorf("component rollback = %q", got.ComponentRollback)
	}
	if !strings.Contains(got.AppRollback, "hook stop failed") {
		t.Errorf("app rollback = %q", got.AppRollback)
	}
	if !got.PortReleased {
		t.Error("started server kept its port after rollback")
	}
}

func childCommand(t *testing.T, mode, logPath string) process.Command {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return process.Command{
		Binary:    binary,
		Args:      []string{"-test.run=^TestHostProcessChild$", "-test.count=1"},
		EnvPolicy: process.EnvEmpty,
		// Children exit with fixed codes through os.Exit, which skips the race detector's
		// end-of-run check, so a race must stop the child with its own distinct code.
		Env:            map[string]string{hostModeEnv: mode, hostLogEnv: logPath, "GORACE": "halt_on_error=1 exitcode=66"},
		MaxOutputBytes: outputBudget,
		GracePeriod:    shutdownBudget,
	}
}

func runChild(t *testing.T, mode, logPath string, wantCode int) *process.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), startupBudget+cleanupBudget)
	defer cancel()
	result, err := process.Run(ctx, childCommand(t, mode, logPath))
	if result == nil {
		t.Fatalf("run %s child: %v", mode, err)
	}
	if result.ExitCode == nil || *result.ExitCode != wantCode || strings.Contains(string(result.Stderr), "DATA RACE") {
		t.Fatalf("%s child exit = %v (err %v), want %d; stderr:\n%s", mode, result.ExitCode, err, wantCode, result.Stderr)
	}
	return result
}

func decodeMarked[T any](t *testing.T, output []byte, marker string) T {
	t.Helper()
	var value T
	for _, line := range strings.Split(string(output), "\n") {
		if rest, ok := strings.CutPrefix(line, marker); ok {
			if err := json.Unmarshal([]byte(rest), &value); err != nil {
				t.Fatal(err)
			}
			return value
		}
	}
	t.Fatalf("child output has no %q line:\n%s", marker, output)
	return value
}

func statusOf(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode
}

type lineStream struct {
	first string
	body  io.Reader
}

func openLine(t *testing.T, ctx context.Context, url string) lineStream {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("GET %s first line: %v", url, err)
	}
	return lineStream{first: first, body: reader}
}

func portOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func printResult(marker string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("%s%s\n", marker, data)
}

func fileLogging(name string) config.ServiceConfig {
	return config.ServiceConfig{Name: name, Logging: logging.Config{
		Level: "info", Format: "json", Output: logging.OutputFile(os.Getenv(hostLogEnv)),
	}}
}

// runServiceHost runs a long-lived host with an owned file logger and two named servers.
func runServiceHost() int {
	cfg := fileLogging("service")
	app, err := bootstrap.NewApp(&cfg, bootstrap.WithGracefulTimeout(shutdownBudget))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	app.Summary.SetWriter(io.Discard)
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer bus.Close()
	streams, err := sse.NewComponent(bus)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	events, err := sse.NewHandler(bus, sse.HandlerConfig{
		Logger: app.Logger, Clock: util.SystemClock{}, WriteTimeout: time.Second, Heartbeat: time.Second,
		Authorize: func(*http.Request) (sse.Access, error) { return sse.Access{Principal: "user", Route: "scope"}, nil },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	public := server.New(&server.Config{Host: "127.0.0.1"}, app.Logger)
	public.Handle("/events", events)
	release := make(chan struct{})
	public.Handle("/slow", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "accepted\n")
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "completed\n")
	}))
	internal := server.New(&server.Config{Host: "127.0.0.1"}, app.Logger)
	internal.Handle("/internal", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, c := range []component.Component{
		streams,
		server.NewComponent(public, server.WithName("public")),
		server.NewComponent(internal, server.WithName("internal")),
	} {
		if err := app.RegisterComponent(c); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	// New requests are already rejected here; the accepted one completes before HTTP drains.
	app.OnBeforeStop(func(context.Context) error {
		close(release)
		return nil
	})
	app.OnReady(func(context.Context) error {
		printResult(hostReadyMarker, hostAddresses{public.ListenAddr().String(), internal.ListenAddr().String()})
		return nil
	})
	err = app.Run(context.Background())
	app.Logger.Info("after-release")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// runFailingTask runs a task that fails while cleanup also fails, using a borrowed logger.
func runFailingTask() int {
	cfg := fileLogging("task")
	borrowed, err := logging.New(&cfg.Logging, "borrowed")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	app, err := bootstrap.NewApp(&cfg, bootstrap.WithLogger(borrowed), bootstrap.WithGracefulTimeout(shutdownBudget))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	app.Summary.SetWriter(io.Discard)
	taskCause, stopCause := errors.New("task failed"), errors.New("cleanup failed")
	app.OnBeforeStop(func(context.Context) error { return stopCause })
	err = app.RunTask(context.Background(), func(context.Context) error { return taskCause })

	var out taskOutcome
	var taskErr *bootstrap.TaskError
	if out.TaskError = errors.As(err, &taskErr); out.TaskError {
		var shutdownErr *bootstrap.ShutdownError
		out.TaskCause = errors.Is(taskErr.Cause, taskCause)
		out.ShutdownError = errors.As(taskErr.Shutdown, &shutdownErr)
		out.ShutdownCause = errors.Is(taskErr.Shutdown, stopCause)
	}
	if err != nil {
		out.Message = err.Error()
	}
	borrowed.Info("borrowed-still-open")
	closeErr := borrowed.Close()
	out.BorrowedCloseOK = closeErr == nil
	out.BorrowedClosed = closeErr != nil
	printResult(hostResultLine, out)
	return taskExitCode
}

// runPartialStart starts a server and a cache, then fails a later component.
func runPartialStart() int {
	cfg := fileLogging("rollback")
	app, err := bootstrap.NewApp(&cfg, bootstrap.WithGracefulTimeout(shutdownBudget))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	app.Summary.SetWriter(io.Discard)
	srv := server.New(&server.Config{Host: "127.0.0.1"}, app.Logger)
	var addr string
	for _, c := range []component.Component{
		server.NewComponent(srv, server.WithName("public")),
		&componenttest.Component{ComponentName: "cache", StopFunc: func(context.Context) error {
			return errors.New("cache stop failed")
		}},
		&componenttest.Component{ComponentName: "database", StartFunc: func(context.Context) error {
			addr = srv.ListenAddr().String()
			return errors.New("database unavailable")
		}},
	} {
		if err := app.RegisterComponent(c); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	app.OnBeforeStop(func(context.Context) error { return errors.New("hook stop failed") })
	err = app.Run(context.Background())

	var out rollbackOutcome
	var startupErr *bootstrap.StartupError
	if errors.As(err, &startupErr) {
		out.Phase = string(startupErr.Phase)
		if startupErr.Rollback != nil {
			out.AppRollback = startupErr.Rollback.Error()
		}
		var startErr *component.StartError
		if out.StartError = errors.As(startupErr.Cause, &startErr); out.StartError {
			out.FailedComponent = startErr.Component
			if startErr.Rollback != nil {
				out.ComponentRollback = startErr.Rollback.Error()
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "Run error = %T %v\n", err, err)
	}
	out.PortReleased = addr != "" && !portOpen(addr)
	printResult(hostResultLine, out)
	return rollbackExitCode
}
