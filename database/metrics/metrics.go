package metrics

import (
	"context"
	"database/sql"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/kbukum/gokit/database"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Register exposes open, in-use and idle gauges, wait count/time, and slow queries. Name is a server-configured identity, unique per database registered with the meter, with at most 128 bytes and no surrounding whitespace. Never use a DSN, tenant or request value. Pool labels are the fixed values primary and reader.
func Register(db *database.DB, meter metric.Meter, name string) (metric.Registration, error) {
	if db == nil || util.IsNil(meter) {
		return nil, apperrors.InvalidInput("metrics", "database and meter are required")
	}
	if name == "" || len(name) > 128 || strings.TrimSpace(name) != name {
		return nil, apperrors.InvalidInput("name", "a database metric identity of 1-128 bytes without surrounding whitespace is required")
	}
	identity := attribute.String("database", name)
	gauges := make([]metric.Int64ObservableGauge, 3)
	counters := make([]metric.Int64ObservableCounter, 2)
	observables := make([]metric.Observable, 0, 6)
	for i, name := range []string{"db.pool.connections.open", "db.pool.connections.in_use", "db.pool.connections.idle"} {
		gauge, err := meter.Int64ObservableGauge(name)
		if err != nil {
			return nil, err
		}
		gauges[i] = gauge
		observables = append(observables, gauge)
	}
	for i, name := range []string{"db.pool.wait.count", "db.queries.slow"} {
		counter, err := meter.Int64ObservableCounter(name)
		if err != nil {
			return nil, err
		}
		counters[i] = counter
		observables = append(observables, counter)
	}
	wait, err := meter.Float64ObservableCounter("db.pool.wait.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	observables = append(observables, wait)
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats, err := db.Stats()
		if err != nil {
			return err
		}
		observe := func(name string, pool sql.DBStats) {
			label := metric.WithAttributes(identity, attribute.String("pool", name))
			for i, value := range []int{pool.OpenConnections, pool.InUse, pool.Idle} {
				observer.ObserveInt64(gauges[i], int64(value), label)
			}
			observer.ObserveInt64(counters[0], pool.WaitCount, label)
			observer.ObserveFloat64(wait, pool.WaitDuration.Seconds(), label)
		}
		observe("primary", stats.Primary)
		if stats.Reader != nil {
			observe("reader", *stats.Reader)
		}
		observer.ObserveInt64(counters[1], int64(stats.SlowQueries), metric.WithAttributes(identity))
		return nil
	}, observables...)
}
