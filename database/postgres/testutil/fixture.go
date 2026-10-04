package testutil

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/moby/moby/client"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Image is the immutable multi-architecture PostgreSQL 17 integration image.
const Image = "postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73"

// Fixture owns one isolated PostgreSQL container. DSN is synthetic test configuration and must not be logged or retained as evidence.
type Fixture struct {
	DSN       string
	terminate func(context.Context) error
	mu        sync.Mutex
	closeErr  error
}

// Start requires a reachable Docker daemon, bounds provisioning to three minutes, and releases any partial container on failure. Each invocation owns a separate container and database.
func Start(ctx context.Context) (*Fixture, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := DockerReady(ctx); err != nil {
		return nil, err
	}
	container, err := tcpostgres.Run(ctx, Image,
		tcpostgres.WithDatabase("fixture"),
		tcpostgres.WithUsername("fixture"),
		tcpostgres.WithPassword(rand.Text()),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	fixture := &Fixture{}
	if container != nil {
		fixture.terminate = func(cleanupCtx context.Context) error { return container.Terminate(cleanupCtx) }
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("provision PostgreSQL fixture: %w", err), fixture.Close(ctx))
	}
	fixture.DSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, errors.Join(err, fixture.Close(ctx))
	}
	return fixture, nil
}

// DockerReady checks the declared Docker prerequisite with a five-second ceiling. It returns a setup error, never a skipped or successful gate.
func DockerReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("docker prerequisite: %w", err)
	}
	_, pingErr := docker.Ping(ctx, client.PingOptions{})
	if err := errors.Join(pingErr, docker.Close()); err != nil {
		return fmt.Errorf("docker prerequisite: %w", err)
	}
	return nil
}

// Close terminates the owned container using a fresh thirty-second budget even when the caller is canceled. Repeated calls return the recorded termination result without repeating termination.
func (f *Fixture) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.terminate == nil {
		return f.closeErr
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	f.closeErr = f.terminate(ctx)
	f.terminate = nil
	return f.closeErr
}
