package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

func newComponent() *database.Component {
	return database.NewComponent(testConfig(), logging.NewDefault("test")).WithDialect(sqlite.Dialect())
}

func TestComponentPublishesOnlyAfterInitializeAndReadiness(t *testing.T) {
	var order []string
	comp := newComponent().
		WithInitialize(func(ctx context.Context, db *database.DB) error {
			order = append(order, "initialize")
			return db.WithContext(ctx).Exec("CREATE TABLE marker (id INTEGER)").Error
		})
	comp.WithReadiness(func(ctx context.Context, db *database.DB) error {
		order = append(order, "readiness")
		if _, err := comp.DB(); !errors.Is(err, database.ErrNotStarted) {
			t.Errorf("pool published before readiness: %v", err)
		}
		return db.WithContext(ctx).Exec("SELECT 1 FROM marker").Error
	})
	if err := comp.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "initialize,readiness" {
		t.Fatalf("check order = %v", order)
	}
	if err := comp.Start(t.Context()); err != nil || len(order) != 2 {
		t.Fatalf("repeated Start is not idempotent: %v %v", err, order)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestComponentFailedStartClosesPoolAndMayRetry(t *testing.T) {
	failure := errors.New("installation mode mismatch")
	var calls atomic.Int32
	var opened *database.DB
	comp := newComponent().WithInitialize(func(_ context.Context, db *database.DB) error {
		if calls.Add(1) == 1 {
			opened = db
			return failure
		}
		return nil
	})
	if err := comp.Start(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("Start = %v, want initialize failure", err)
	}
	if err := opened.PingContext(t.Context()); err == nil {
		t.Fatal("failed Start left its pool open")
	}
	if _, err := comp.DB(); !errors.Is(err, database.ErrNotStarted) {
		t.Fatalf("failed Start published: %v", err)
	}
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("retry after failed Start: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestComponentStopIsTerminalAndRecorded(t *testing.T) {
	comp := newComponent()
	if err := comp.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	db := mustDB(t, comp)
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(t.Context()); err == nil {
		t.Fatal("Stop did not close the pool")
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("repeated Stop = %v", err)
	}
	if err := comp.Start(t.Context()); !errors.Is(err, database.ErrStopped) {
		t.Fatalf("Start after Stop = %v", err)
	}
	if _, err := comp.DB(); !errors.Is(err, database.ErrStopped) {
		t.Fatalf("DB after Stop = %v", err)
	}
	if h := comp.Health(t.Context()); h.Status != component.StatusUnhealthy {
		t.Fatalf("Health after Stop = %+v", h)
	}
}

func TestComponentStopDuringInitializationPreventsPublication(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var opened *database.DB
	comp := newComponent().WithInitialize(func(_ context.Context, db *database.DB) error {
		opened = db
		close(entered)
		<-release
		return nil
	})
	started := make(chan error, 1)
	go func() { started <- comp.Start(context.Background()) }()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- comp.Stop(context.Background()) }()
	if !eventually(func() bool { return comp.Health(t.Context()).Message == "database stopped" }) {
		t.Fatal("Stop did not mark the component stopped while Start was initializing")
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before the in-flight Start released its pool: %v", err)
	default:
	}
	close(release)
	if err := <-started; !errors.Is(err, database.ErrStopped) {
		t.Fatalf("in-flight Start = %v, want ErrStopped", err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := opened.PingContext(t.Context()); err == nil {
		t.Fatal("late pool was published or left open")
	}
	if _, err := comp.DB(); !errors.Is(err, database.ErrStopped) {
		t.Fatalf("DB = %v", err)
	}
}

func TestComponentStopRecordsInflightStartCleanupFailure(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	comp := newComponent().WithInitialize(func(_ context.Context, db *database.DB) error {
		pool, err := db.GormDB.DB()
		if err != nil {
			return err
		}
		t.Cleanup(func() { _ = pool.Close() })
		// An opaque pool makes the late close fail, standing in for a driver close error.
		db.GormDB.Statement.ConnPool = struct{ gorm.ConnPool }{pool}
		close(entered)
		<-release
		return nil
	})
	started := make(chan error, 1)
	go func() { started <- comp.Start(context.Background()) }()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- comp.Stop(context.Background()) }()
	if !eventually(func() bool { return comp.Health(t.Context()).Message == "database stopped" }) {
		t.Fatal("Stop did not mark the component stopped")
	}
	close(release)
	startErr := <-started
	if !errors.Is(startErr, database.ErrStopped) || !errors.Is(startErr, gorm.ErrInvalidDB) {
		t.Fatalf("in-flight Start = %v, want ErrStopped joined with the close failure", startErr)
	}
	if err := <-stopped; !errors.Is(err, gorm.ErrInvalidDB) {
		t.Fatalf("Stop = %v, want the late close failure", err)
	}
	if err := comp.Stop(t.Context()); !errors.Is(err, gorm.ErrInvalidDB) {
		t.Fatalf("repeated Stop = %v, want the recorded close failure", err)
	}
}

func TestComponentLifecycleErrorsAreClassified(t *testing.T) {
	comp := newComponent()
	_, err := comp.DB()
	if app := apperrors.Normalize(err); !errors.Is(err, database.ErrNotStarted) || app.Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatalf("DB before Start = %v (%s)", err, app.Code)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{"Start": comp.Start(t.Context()), "DB": func() error { _, err := comp.DB(); return err }()} {
		app := apperrors.Normalize(err)
		if !errors.Is(err, database.ErrStopped) || app.Code != apperrors.ErrCodeServiceUnavailable || app.Retryable {
			t.Fatalf("%s after Stop = %v (%s, retryable=%v)", name, err, app.Code, app.Retryable)
		}
	}
}

func TestComponentStopWaitIsBoundedByContext(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	comp := newComponent().WithInitialize(func(context.Context, *database.DB) error {
		close(entered)
		<-release
		return nil
	})
	started := make(chan error, 1)
	go func() { started <- comp.Start(context.Background()) }()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := comp.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v, want its deadline", err)
	}
	close(release)
	if err := <-started; !errors.Is(err, database.ErrStopped) {
		t.Fatalf("Start = %v", err)
	}
}

func TestComponentConcurrentStartWaitsForInflightAttempt(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	comp := newComponent().WithInitialize(func(context.Context, *database.DB) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	})
	first := make(chan error, 1)
	go func() { first <- comp.Start(context.Background()) }()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := comp.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Start = %v, want its deadline", err)
	}
	second := make(chan error, 1)
	go func() { second <- comp.Start(context.Background()) }()
	close(release)
	if err := errors.Join(<-first, <-second); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("initialize ran %d times", calls.Load())
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestComponentHealthReportsDriftWithSafeBoundedMessage(t *testing.T) {
	var drift atomic.Bool
	comp := newComponent().WithName("orders-db").WithReadiness(func(ctx context.Context, db *database.DB) error {
		if drift.Load() {
			return database.Failure(errors.New("password=secret relation missing"))
		}
		return db.PingContext(ctx)
	})
	if err := comp.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = comp.Stop(context.Background()) })
	if h := comp.Health(t.Context()); h.Status != component.StatusHealthy || h.Name != "orders-db" {
		t.Fatalf("Health = %+v", h)
	}
	drift.Store(true)
	h := comp.Health(t.Context())
	if h.Status != component.StatusUnhealthy || strings.Contains(h.Message, "secret") || !strings.HasPrefix(h.Message, "not ready: ") {
		t.Fatalf("Health after drift = %+v", h)
	}
	drift.Store(false)
	comp.WithReadiness(func(ctx context.Context, _ *database.DB) error {
		<-ctx.Done()
		return ctx.Err()
	})
	begin := time.Now()
	if h := comp.Health(t.Context()); h.Status != component.StatusUnhealthy || !strings.Contains(h.Message, "TIMEOUT") {
		t.Fatalf("Health with stuck readiness = %+v", h)
	}
	if elapsed := time.Since(begin); elapsed > database.HealthTimeout+time.Second {
		t.Fatalf("Health took %v", elapsed)
	}
}

func TestComponentNamedInstancesRegisterTogether(t *testing.T) {
	registry := component.NewRegistry()
	if err := registry.Register(newComponent().WithName("orders-db")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(newComponent().WithName("billing-db")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(newComponent().WithName("orders-db")); err == nil {
		t.Fatal("duplicate component name registered")
	}
}

func TestComponentLifecycleIsRaceFree(t *testing.T) {
	comp := newComponent().WithReadiness(func(ctx context.Context, db *database.DB) error { return db.PingContext(ctx) })
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _ = comp.Start(context.Background()) })
		wg.Go(func() { _ = comp.Health(context.Background()) })
		wg.Go(func() { _, _ = comp.DB() })
	}
	wg.Go(func() { _ = comp.Stop(context.Background()) })
	wg.Wait()
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := comp.DB(); !errors.Is(err, database.ErrStopped) {
		t.Fatalf("DB = %v", err)
	}
}

func eventually(condition func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
