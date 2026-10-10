package password

import (
	"context"
	"sync"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// BenchmarkPoolEnvelope measures the RunLab admission profile (2 workers, queue 8, 250 ms wait) with a full burst of
// concurrent verifications: the slowest admitted operation including queue wait, busy rejections, and how long Close
// takes to drain a full burst. Targets: 2 s per operation, 5 s drain.
func BenchmarkPoolEnvelope(b *testing.B) {
	const workers, queue, burst = 2, 8, 2 + 8
	for name, cfg := range map[string]Config{
		"argon2id-issued":  {},
		"argon2id-maximum": {Argon2Memory: maxArgon2Memory, Argon2Time: maxArgon2Time, Argon2Threads: 8},
	} {
		b.Run(name, func(b *testing.B) {
			hasher, err := NewHasher(cfg)
			if err != nil {
				b.Fatal(err)
			}
			hash, err := hasher.Hash("benchmark-password")
			if err != nil {
				b.Fatal(err)
			}
			var slowest, drain time.Duration
			busy := 0
			for range b.N {
				pool, err := NewPool(hasher, PoolConfig{Workers: workers, Queue: queue, MaxWait: 250 * time.Millisecond})
				if err != nil {
					b.Fatal(err)
				}
				var mu sync.Mutex
				var wg sync.WaitGroup
				for range burst {
					wg.Go(func() {
						start := time.Now()
						err := pool.Verify(context.Background(), "benchmark-password", hash)
						elapsed := time.Since(start)
						mu.Lock()
						defer mu.Unlock()
						switch {
						case err == nil:
							slowest = max(slowest, elapsed)
						case apperrors.Normalize(err).Code == apperrors.ErrCodeRateLimited:
							busy++
						default:
							b.Error(err)
						}
					})
				}
				wg.Wait()
				for range burst {
					go func() { _ = pool.Verify(context.Background(), "benchmark-password", hash) }()
				}
				time.Sleep(time.Millisecond)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				start := time.Now()
				err = pool.Close(ctx)
				drain = max(drain, time.Since(start))
				cancel()
				if err != nil {
					b.Fatalf("drain exceeded 5 s: %v", err)
				}
			}
			b.ReportMetric(float64(slowest.Milliseconds()), "slowest-ms")
			b.ReportMetric(float64(drain.Milliseconds()), "drain-ms")
			b.ReportMetric(float64(busy)/float64(b.N), "busy/burst")
			if slowest > 2*time.Second {
				b.Fatalf("slowest admitted verification %v exceeds 2 s", slowest)
			}
		})
	}
}
