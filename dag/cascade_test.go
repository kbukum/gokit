package dag

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

// testProvider is a simple RequestResponse for testing cascades.
type testProvider[I, O any] struct {
	name      string
	available bool
	execFn    func(ctx context.Context, input I) (O, error)
}

func (p *testProvider[I, O]) Name() string                       { return p.name }
func (p *testProvider[I, O]) IsAvailable(_ context.Context) bool { return p.available }
func (p *testProvider[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return p.execFn(ctx, input)
}

func newTestProvider[I, O any](name string, fn func(ctx context.Context, input I) (O, error)) provider.RequestResponse[I, O] {
	return &testProvider[I, O]{
		name:      name,
		available: true,
		execFn:    fn,
	}
}

func newTestProviderWithMeta[I, O any](name string, meta provider.Meta, fn func(ctx context.Context, input I) (O, error)) provider.RequestResponse[I, O] {
	p := newTestProvider[I, O](name, fn)
	return provider.WithMeta[I, O](p, meta)
}

// --- Test types ---

type analysisInput struct {
	Content  string
	HasVideo bool
	HasAudio bool
}

type analysisResult struct {
	Confidence float64
	Scores     map[string]float64
}

func mergeResults(a, b analysisResult) analysisResult {
	merged := analysisResult{
		Confidence: a.Confidence,
		Scores:     make(map[string]float64),
	}
	for k, v := range a.Scores {
		merged.Scores[k] = v
	}
	for k, v := range b.Scores {
		merged.Scores[k] = v
	}
	// Take the higher confidence.
	if b.Confidence > merged.Confidence {
		merged.Confidence = b.Confidence
	}
	return merged
}

func TestCascade_BasicExecution(t *testing.T) {
	p1 := newTestProvider[analysisInput, analysisResult]("tier1", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{Confidence: 0.5, Scores: map[string]float64{"metadata": 0.5}}, nil
	})
	p2 := newTestProvider[analysisInput, analysisResult]("tier2", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{Confidence: 0.8, Scores: map[string]float64{"frequency": 0.8}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("metadata", p1)
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("frequency", p2)
		}).
		MergeStrategy(mergeResults).
		Build()

	result, trace := cascade.Execute(context.Background(), analysisInput{Content: "test"})

	if len(trace.StagesExecuted) != 2 {
		t.Errorf("expected 2 stages executed, got %d: %v", len(trace.StagesExecuted), trace.StagesExecuted)
	}
	if result.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", result.Confidence)
	}
	if len(trace.NodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(trace.NodeResults))
	}
}

func TestCascade_EarlyExit(t *testing.T) {
	p1 := newTestProvider[analysisInput, analysisResult]("metadata", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{Confidence: 0.99, Scores: map[string]float64{"metadata": 0.99}}, nil
	})
	p2 := newTestProvider[analysisInput, analysisResult]("frequency", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		t.Fatal("tier2 should not execute on early exit")
		return analysisResult{}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("metadata", p1)
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("frequency", p2)
		}).
		MergeStrategy(mergeResults).
		Build()

	result, trace := cascade.Execute(context.Background(), analysisInput{})

	if !trace.EarlyExit {
		t.Error("expected early exit")
	}
	if trace.ExitedAtStage != "tier1" {
		t.Errorf("expected exit at tier1, got %q", trace.ExitedAtStage)
	}
	if len(trace.StagesExecuted) != 1 {
		t.Errorf("expected 1 stage executed, got %d", len(trace.StagesExecuted))
	}
	if len(trace.StagesSkipped) != 1 {
		t.Errorf("expected 1 stage skipped, got %d", len(trace.StagesSkipped))
	}
	if result.Confidence != 0.99 {
		t.Errorf("expected confidence 0.99, got %f", result.Confidence)
	}
}

func TestCascade_EarlyExitWithFinalStage(t *testing.T) {
	tier1Ran := false
	fusionRan := false

	p1 := newTestProvider[analysisInput, analysisResult]("metadata", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		tier1Ran = true
		return analysisResult{Confidence: 0.99, Scores: map[string]float64{"metadata": 0.99}}, nil
	})
	pFusion := newTestProvider[analysisInput, analysisResult]("fusion", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		fusionRan = true
		return analysisResult{Confidence: 0.99, Scores: map[string]float64{"fusion": 1.0}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("metadata", p1)
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("never", newTestProvider[analysisInput, analysisResult]("never", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				t.Fatal("should not run")
				return analysisResult{}, nil
			}))
		}).
		FinalStage("fusion", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("fusion", pFusion)
		}).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if !tier1Ran {
		t.Error("tier1 should have run")
	}
	if !fusionRan {
		t.Error("fusion (final stage) should always run")
	}
	if !trace.EarlyExit {
		t.Error("expected early exit")
	}
	// Final stage is always included in StagesExecuted.
	found := false
	for _, s := range trace.StagesExecuted {
		if s == "fusion" {
			found = true
		}
	}
	if !found {
		t.Error("fusion should be in StagesExecuted")
	}
}

func TestCascade_ConditionalNodes(t *testing.T) {
	videoRan := false
	audioRan := false

	pVideo := newTestProvider[analysisInput, analysisResult]("spatial", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		videoRan = true
		return analysisResult{Confidence: 0.8, Scores: map[string]float64{"spatial": 0.8}}, nil
	})
	pAudio := newTestProvider[analysisInput, analysisResult]("wav2vec", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		audioRan = true
		return analysisResult{Confidence: 0.7, Scores: map[string]float64{"wav2vec": 0.7}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("analyzers", func(b *StageBuilder[analysisInput, analysisResult], input analysisInput) {
			if input.HasVideo {
				b.AddNode("spatial", pVideo)
			}
			if input.HasAudio {
				b.AddNode("wav2vec", pAudio)
			}
		}).
		MergeStrategy(mergeResults).
		Build()

	// Video only.
	cascade.Execute(context.Background(), analysisInput{HasVideo: true, HasAudio: false})
	if !videoRan {
		t.Error("video analyzer should run")
	}
	if audioRan {
		t.Error("audio analyzer should NOT run")
	}

	// Reset.
	videoRan = false
	audioRan = false

	// Both.
	cascade.Execute(context.Background(), analysisInput{HasVideo: true, HasAudio: true})
	if !videoRan || !audioRan {
		t.Error("both analyzers should run")
	}

	// Reset.
	videoRan = false
	audioRan = false

	// Neither.
	_, trace := cascade.Execute(context.Background(), analysisInput{HasVideo: false, HasAudio: false})
	if videoRan || audioRan {
		t.Error("no analyzers should run")
	}
	if len(trace.NodeResults) != 0 {
		t.Errorf("expected 0 node results, got %d", len(trace.NodeResults))
	}
}

func TestCascade_ParallelWithinStage(t *testing.T) {
	startCh := make(chan struct{})
	p1 := newTestProvider[analysisInput, analysisResult]("a", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		<-startCh // Wait until both goroutines are running.
		return analysisResult{Confidence: 0.5, Scores: map[string]float64{"a": 0.5}}, nil
	})
	p2 := newTestProvider[analysisInput, analysisResult]("b", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		<-startCh
		return analysisResult{Confidence: 0.6, Scores: map[string]float64{"b": 0.6}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("parallel", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("a", p1)
			b.AddNode("b", p2)
		}).
		MergeStrategy(mergeResults).
		Build()

	// Close channel to unblock both goroutines simultaneously.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(startCh)
	}()

	result, trace := cascade.Execute(context.Background(), analysisInput{})

	if len(trace.NodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(trace.NodeResults))
	}
	if result.Confidence != 0.6 {
		t.Errorf("expected confidence 0.6, got %f", result.Confidence)
	}
}

func TestCascade_InternalEdges(t *testing.T) {
	order := make([]string, 0, 2)

	pDire := newTestProvider[analysisInput, analysisResult]("dire", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		order = append(order, "dire")
		return analysisResult{Confidence: 0.7, Scores: map[string]float64{"dire": 0.7}}, nil
	})
	pTemporal := newTestProvider[analysisInput, analysisResult]("temporal", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		order = append(order, "temporal")
		return analysisResult{Confidence: 0.8, Scores: map[string]float64{"temporal": 0.8}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("gpu", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("dire", pDire)
			b.AddNode("temporal", pTemporal)
			b.Edge("dire", "temporal") // temporal depends on dire
		}).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if len(order) != 2 || order[0] != "dire" || order[1] != "temporal" {
		t.Errorf("expected [dire, temporal], got %v", order)
	}
	if len(trace.NodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(trace.NodeResults))
	}
}

func TestCascade_StageFailure_Abort(t *testing.T) {
	pFail := newTestProvider[analysisInput, analysisResult]("failing", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{}, errors.New("analyzer crashed")
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("failing", pFail)
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("should-not-run", newTestProvider[analysisInput, analysisResult]("noop", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				t.Fatal("should not run after abort")
				return analysisResult{}, nil
			}))
		}).
		OnStageFailure(Abort()).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if trace.Error == nil {
		t.Error("expected error on abort")
	}
	if len(trace.StagesExecuted) != 1 {
		t.Errorf("expected 1 stage executed, got %d", len(trace.StagesExecuted))
	}
}

func TestCascade_StageFailure_SkipToFinal(t *testing.T) {
	fusionRan := false

	pFail := newTestProvider[analysisInput, analysisResult]("failing", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{}, errors.New("crashed")
	})
	pFusion := newTestProvider[analysisInput, analysisResult]("fusion", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		fusionRan = true
		return analysisResult{Confidence: 0.3, Scores: map[string]float64{"fusion": 0.3}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("failing", pFail)
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("never", newTestProvider[analysisInput, analysisResult]("noop", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				t.Fatal("should not run on skip-to-final")
				return analysisResult{}, nil
			}))
		}).
		FinalStage("fusion", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("fusion", pFusion)
		}).
		OnStageFailure(SkipToFinal()).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if !fusionRan {
		t.Error("fusion should run on skip-to-final")
	}
	if len(trace.StagesSkipped) != 1 {
		t.Errorf("expected 1 skipped stage, got %d: %v", len(trace.StagesSkipped), trace.StagesSkipped)
	}
}

func TestCascade_ContinueWithPartial(t *testing.T) {
	goodRan := false

	pFail := newTestProvider[analysisInput, analysisResult]("failing", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{}, errors.New("crashed")
	})
	pGood := newTestProvider[analysisInput, analysisResult]("good", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		goodRan = true
		return analysisResult{Confidence: 0.7, Scores: map[string]float64{"good": 0.7}}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("mixed", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("failing", pFail)
			b.AddNode("good", pGood)
			b.OnFailure(ContinueWithPartial())
		}).
		MergeStrategy(mergeResults).
		Build()

	result, trace := cascade.Execute(context.Background(), analysisInput{})

	if !goodRan {
		t.Error("good provider should run")
	}
	if result.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", result.Confidence)
	}
	if trace.Error != nil {
		t.Errorf("expected no error with ContinueWithPartial, got %v", trace.Error)
	}
	// Check both nodes are in traces.
	if len(trace.NodeResults) != 2 {
		t.Errorf("expected 2 node results, got %d", len(trace.NodeResults))
	}
	if trace.NodeResults["failing"].Status != StatusFailed {
		t.Errorf("failing node status = %q, want %q", trace.NodeResults["failing"].Status, StatusFailed)
	}
	if trace.NodeResults["good"].Status != StatusCompleted {
		t.Errorf("good node status = %q, want %q", trace.NodeResults["good"].Status, StatusCompleted)
	}
}

func TestCascade_ProviderMeta_CostTracking(t *testing.T) {
	p1 := newTestProviderWithMeta[analysisInput, analysisResult]("metadata",
		provider.Meta{"cost": 0.0, "latency_ms": 1.0},
		func(_ context.Context, _ analysisInput) (analysisResult, error) {
			return analysisResult{Confidence: 0.5, Scores: map[string]float64{"metadata": 0.5}}, nil
		})
	p2 := newTestProviderWithMeta[analysisInput, analysisResult]("frequency",
		provider.Meta{"cost": 0.01, "latency_ms": 50.0},
		func(_ context.Context, _ analysisInput) (analysisResult, error) {
			return analysisResult{Confidence: 0.8, Scores: map[string]float64{"frequency": 0.8}}, nil
		})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("metadata", p1)
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("frequency", p2)
		}).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if trace.TotalCost != 0.01 {
		t.Errorf("expected total cost 0.01, got %f", trace.TotalCost)
	}

	// Verify meta is on node traces.
	if cost, ok := trace.NodeResults["metadata"].Meta.Float("cost"); !ok || cost != 0.0 {
		t.Errorf("metadata cost meta not propagated")
	}
	if cost, ok := trace.NodeResults["frequency"].Meta.Float("cost"); !ok || cost != 0.01 {
		t.Errorf("frequency cost meta not propagated")
	}
}

func TestCascade_OrderBy(t *testing.T) {
	var mu sync.Mutex
	order := make([]string, 0, 3)

	makeProvider := func(name string, cost float64) provider.RequestResponse[analysisInput, analysisResult] {
		return newTestProviderWithMeta[analysisInput, analysisResult](name,
			provider.Meta{"cost": cost},
			func(_ context.Context, _ analysisInput) (analysisResult, error) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return analysisResult{Confidence: 0.5, Scores: map[string]float64{name: 0.5}}, nil
			})
	}

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("parallel", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			// Add in reverse cost order.
			b.AddNode("expensive", makeProvider("expensive", 1.0))
			b.AddNode("cheap", makeProvider("cheap", 0.01))
			b.AddNode("free", makeProvider("free", 0.0))
		}).
		OrderNodesBy(OrderByCost()).
		MaxConcurrency(1). // Force sequential to test ordering.
		MergeStrategy(mergeResults).
		Build()

	cascade.Execute(context.Background(), analysisInput{})

	if len(order) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(order))
	}
	if order[0] != "free" || order[1] != "cheap" || order[2] != "expensive" {
		t.Errorf("expected [free, cheap, expensive], got %v", order)
	}
}

func TestCascade_Timeout(t *testing.T) {
	pSlow := newTestProvider[analysisInput, analysisResult]("slow", func(ctx context.Context, _ analysisInput) (analysisResult, error) {
		select {
		case <-ctx.Done():
			return analysisResult{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return analysisResult{Confidence: 1.0}, nil
		}
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("slow-stage", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("slow", pSlow)
			b.Timeout(50 * time.Millisecond)
		}).
		OnStageFailure(Abort()).
		Build()

	start := time.Now()
	_, trace := cascade.Execute(context.Background(), analysisInput{})
	dur := time.Since(start)

	if dur > 500*time.Millisecond {
		t.Errorf("expected timeout within ~50ms, took %v", dur)
	}
	if trace.Error == nil {
		t.Error("expected error from timeout")
	}
}

func TestCascade_EmptyStages(t *testing.T) {
	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("empty1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			// No nodes added.
		}).
		Stage("empty2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			// No nodes added.
		}).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if trace.Error != nil {
		t.Errorf("empty cascade should not error, got %v", trace.Error)
	}
	if len(trace.StagesExecuted) != 2 {
		t.Errorf("expected 2 stages executed, got %d", len(trace.StagesExecuted))
	}
}

func TestCascade_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p1 := newTestProvider[analysisInput, analysisResult]("p1", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		cancel() // Cancel after first stage.
		return analysisResult{Confidence: 0.5}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("p1", p1)
			b.AdvanceWhen(func(r analysisResult) bool { return true }) // Always advance.
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("never", newTestProvider[analysisInput, analysisResult]("noop", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				t.Fatal("should not run after cancellation")
				return analysisResult{}, nil
			}))
		}).
		Build()

	_, trace := cascade.Execute(ctx, analysisInput{})

	if len(trace.StagesSkipped) != 1 {
		t.Errorf("expected 1 skipped stage, got %d", len(trace.StagesSkipped))
	}
}

func TestCascade_TraceCompleteness(t *testing.T) {
	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("metadata", newTestProviderWithMeta[analysisInput, analysisResult]("metadata",
				provider.Meta{"cost": 0.0},
				func(_ context.Context, _ analysisInput) (analysisResult, error) {
					return analysisResult{Confidence: 0.5}, nil
				}))
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("frequency", newTestProviderWithMeta[analysisInput, analysisResult]("frequency",
				provider.Meta{"cost": 0.01},
				func(_ context.Context, _ analysisInput) (analysisResult, error) {
					return analysisResult{Confidence: 0.96}, nil
				}))
			b.AdvanceWhen(func(r analysisResult) bool { return r.Confidence < 0.95 })
		}).
		Stage("tier3", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("never", newTestProvider[analysisInput, analysisResult]("never", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				t.Fatal("tier3 should be skipped")
				return analysisResult{}, nil
			}))
		}).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	// Check stages.
	if len(trace.StagesExecuted) != 2 {
		t.Errorf("StagesExecuted = %v, want 2", trace.StagesExecuted)
	}
	if len(trace.StagesSkipped) != 1 {
		t.Errorf("StagesSkipped = %v, want 1", trace.StagesSkipped)
	}

	// Check early exit.
	if !trace.EarlyExit {
		t.Error("expected EarlyExit = true")
	}
	if trace.ExitedAtStage != "tier2" {
		t.Errorf("ExitedAtStage = %q, want tier2", trace.ExitedAtStage)
	}

	// Check node results.
	if trace.NodeResults["metadata"].Status != StatusCompleted {
		t.Error("metadata should be completed")
	}
	if trace.NodeResults["frequency"].Status != StatusCompleted {
		t.Error("frequency should be completed")
	}

	// Check total cost.
	if trace.TotalCost != 0.01 {
		t.Errorf("TotalCost = %f, want 0.01", trace.TotalCost)
	}

	// Check duration is positive.
	if trace.TotalDuration <= 0 {
		t.Error("TotalDuration should be positive")
	}
}

func TestCascade_OrderByLatency(t *testing.T) {
	var mu sync.Mutex
	order := make([]string, 0, 2)

	makeProv := func(name string, latency float64) provider.RequestResponse[analysisInput, analysisResult] {
		return newTestProviderWithMeta[analysisInput, analysisResult](name,
			provider.Meta{"latency_ms": latency},
			func(_ context.Context, _ analysisInput) (analysisResult, error) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return analysisResult{Confidence: 0.5, Scores: map[string]float64{name: 0.5}}, nil
			})
	}

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("s", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("slow", makeProv("slow", 5000))
			b.AddNode("fast", makeProv("fast", 1))
		}).
		OrderNodesBy(OrderByLatency()).
		MaxConcurrency(1).
		MergeStrategy(mergeResults).
		Build()

	cascade.Execute(context.Background(), analysisInput{})

	if len(order) != 2 || order[0] != "fast" || order[1] != "slow" {
		t.Errorf("expected [fast, slow], got %v", order)
	}
}

func TestCascade_WeightedScore(t *testing.T) {
	var mu sync.Mutex
	order := make([]string, 0, 2)

	makeProv := func(name string, cost, latency float64) provider.RequestResponse[analysisInput, analysisResult] {
		return newTestProviderWithMeta[analysisInput, analysisResult](name,
			provider.Meta{"cost": cost, "latency_ms": latency},
			func(_ context.Context, _ analysisInput) (analysisResult, error) {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return analysisResult{Confidence: 0.5, Scores: map[string]float64{name: 0.5}}, nil
			})
	}

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("s", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("expensive-fast", makeProv("expensive-fast", 1.0, 1))
			b.AddNode("cheap-slow", makeProv("cheap-slow", 0.01, 1000))
		}).
		OrderNodesBy(WeightedScore(map[string]float64{
			"cost":       0.7,
			"latency_ms": 0.3,
		})).
		MaxConcurrency(1).
		MergeStrategy(mergeResults).
		Build()

	cascade.Execute(context.Background(), analysisInput{})

	if len(order) != 2 || order[0] != "expensive-fast" || order[1] != "cheap-slow" {
		t.Errorf("expected [expensive-fast, cheap-slow], got %v", order)
	}
}

func TestCascade_ContinueOnFailurePolicy(t *testing.T) {
	tier2Ran := false

	pFail := newTestProvider[analysisInput, analysisResult]("failing", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		return analysisResult{}, errors.New("crashed")
	})
	pNext := newTestProvider[analysisInput, analysisResult]("next", func(_ context.Context, _ analysisInput) (analysisResult, error) {
		tier2Ran = true
		return analysisResult{Confidence: 0.6}, nil
	})

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("tier1", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("failing", pFail)
		}).
		Stage("tier2", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("next", pNext)
		}).
		OnStageFailure(ContinueOnFailure()).
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	if !tier2Ran {
		t.Error("tier2 should run with ContinueOnFailure policy")
	}
	if trace.Error != nil {
		t.Errorf("expected no final error, got %v", trace.Error)
	}
}

func TestCascade_SequentialContextCancel(t *testing.T) {
	// MaxConcurrency(1) forces sequential path. Cancel after first node.
	ctx, cancel := context.WithCancel(context.Background())
	nodeCount := 0

	makeNode := func(name string) provider.RequestResponse[analysisInput, analysisResult] {
		return newTestProvider[analysisInput, analysisResult](name, func(_ context.Context, _ analysisInput) (analysisResult, error) {
			nodeCount++
			if nodeCount == 1 {
				cancel() // Cancel after first node.
			}
			return analysisResult{Confidence: 0.5, Scores: map[string]float64{name: 0.5}}, nil
		})
	}

	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("stage", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("first", makeNode("first"))
			b.AddNode("second", makeNode("second"))
			b.AddNode("third", makeNode("third"))
		}).
		MaxConcurrency(1). // Force sequential.
		MergeStrategy(mergeResults).
		Build()

	_, trace := cascade.Execute(ctx, analysisInput{})

	if nodeCount != 1 {
		t.Errorf("expected only 1 node to execute, got %d", nodeCount)
	}
	// Second and third should be skipped.
	skipped := 0
	for _, nt := range trace.NodeResults {
		if nt.Status == StatusSkipped {
			skipped++
		}
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped nodes, got %d", skipped)
	}
}

func TestCascade_SequentialAllFailWithPartial(t *testing.T) {
	cascade := NewCascade[analysisInput, analysisResult]().
		Stage("failing", func(b *StageBuilder[analysisInput, analysisResult], _ analysisInput) {
			b.AddNode("f1", newTestProvider[analysisInput, analysisResult]("f1", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				return analysisResult{}, errors.New("fail1")
			}))
			b.AddNode("f2", newTestProvider[analysisInput, analysisResult]("f2", func(_ context.Context, _ analysisInput) (analysisResult, error) {
				return analysisResult{}, errors.New("fail2")
			}))
			b.OnFailure(ContinueWithPartial())
		}).
		MaxConcurrency(1). // Force sequential.
		OnStageFailure(ContinueOnFailure()).
		Build()

	_, trace := cascade.Execute(context.Background(), analysisInput{})

	// With ContinueWithPartial, stage should not propagate error.
	if trace.Error != nil {
		t.Errorf("expected no error with ContinueWithPartial, got %v", trace.Error)
	}
}
