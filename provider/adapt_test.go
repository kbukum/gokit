package provider

import (
	"context"
	"errors"
	"testing"
)

// --- test helpers ---

type backendInput struct {
	Raw string
}

type backendOutput struct {
	Data string
}

type domainInput struct {
	Query string
}

type domainOutput struct {
	Result string
}

type stubBackend struct {
	name      string
	available bool
	execFn    func(ctx context.Context, in backendInput) (backendOutput, error)
}

func (s *stubBackend) Name() string                       { return s.name }
func (s *stubBackend) IsAvailable(_ context.Context) bool { return s.available }
func (s *stubBackend) Execute(ctx context.Context, in backendInput) (backendOutput, error) {
	return s.execFn(ctx, in)
}

// --- tests ---

func TestAdapt_BasicMapping(t *testing.T) {
	backend := &stubBackend{
		name:      "test-backend",
		available: true,
		execFn: func(_ context.Context, in backendInput) (backendOutput, error) {
			return backendOutput{Data: "result:" + in.Raw}, nil
		},
	}

	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		backend,
		"domain-service",
		func(_ context.Context, in domainInput) (backendInput, error) {
			return backendInput{Raw: in.Query}, nil
		},
		func(out backendOutput) (domainOutput, error) {
			return domainOutput{Result: out.Data}, nil
		},
	)

	if adapted.Name() != "domain-service" {
		t.Fatalf("expected name 'domain-service', got %q", adapted.Name())
	}
	if !adapted.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}

	result, err := adapted.Execute(context.Background(), domainInput{Query: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "result:hello" {
		t.Fatalf("expected 'result:hello', got %q", result.Result)
	}
}

func TestAdapt_MapInError(t *testing.T) {
	backend := &stubBackend{
		name:      "test",
		available: true,
		execFn: func(_ context.Context, _ backendInput) (backendOutput, error) {
			t.Fatal("backend should not be called when mapIn fails")
			return backendOutput{}, nil
		},
	}

	mapInErr := errors.New("invalid input")
	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		backend,
		"err-service",
		func(_ context.Context, _ domainInput) (backendInput, error) {
			return backendInput{}, mapInErr
		},
		func(out backendOutput) (domainOutput, error) {
			return domainOutput{Result: out.Data}, nil
		},
	)

	_, err := adapted.Execute(context.Background(), domainInput{Query: "x"})
	if !errors.Is(err, mapInErr) {
		t.Fatalf("expected mapIn error, got %v", err)
	}
}

func TestAdapt_MapOutError(t *testing.T) {
	backend := &stubBackend{
		name:      "test",
		available: true,
		execFn: func(_ context.Context, in backendInput) (backendOutput, error) {
			return backendOutput{Data: in.Raw}, nil
		},
	}

	mapOutErr := errors.New("bad output")
	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		backend,
		"out-err",
		func(_ context.Context, in domainInput) (backendInput, error) {
			return backendInput{Raw: in.Query}, nil
		},
		func(_ backendOutput) (domainOutput, error) {
			return domainOutput{}, mapOutErr
		},
	)

	_, err := adapted.Execute(context.Background(), domainInput{Query: "x"})
	if !errors.Is(err, mapOutErr) {
		t.Fatalf("expected mapOut error, got %v", err)
	}
}

func TestAdapt_BackendError(t *testing.T) {
	backendErr := errors.New("backend failed")
	backend := &stubBackend{
		name:      "test",
		available: true,
		execFn: func(_ context.Context, _ backendInput) (backendOutput, error) {
			return backendOutput{}, backendErr
		},
	}

	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		backend,
		"be-err",
		func(_ context.Context, in domainInput) (backendInput, error) {
			return backendInput{Raw: in.Query}, nil
		},
		func(out backendOutput) (domainOutput, error) {
			return domainOutput{Result: out.Data}, nil
		},
	)

	_, err := adapted.Execute(context.Background(), domainInput{Query: "x"})
	if !errors.Is(err, backendErr) {
		t.Fatalf("expected backend error, got %v", err)
	}
}

func TestAdapt_IsAvailableDelegates(t *testing.T) {
	backend := &stubBackend{
		name:      "test",
		available: false,
		execFn:    nil,
	}

	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		backend,
		"avail-test",
		func(_ context.Context, in domainInput) (backendInput, error) {
			return backendInput{Raw: in.Query}, nil
		},
		func(out backendOutput) (domainOutput, error) {
			return domainOutput{Result: out.Data}, nil
		},
	)

	if adapted.IsAvailable(context.Background()) {
		t.Fatal("expected not available")
	}
}

func TestAdapt_ComposesWithResilience(t *testing.T) {
	calls := 0
	backend := &stubBackend{
		name:      "test",
		available: true,
		execFn: func(_ context.Context, in backendInput) (backendOutput, error) {
			calls++
			return backendOutput{Data: in.Raw}, nil
		},
	}

	// Wrap backend with resilience first, then adapt
	resilient := WithResilience[backendInput, backendOutput](backend, ResilienceConfig{})

	adapted := Adapt[domainInput, domainOutput, backendInput, backendOutput](
		resilient,
		"composed",
		func(_ context.Context, in domainInput) (backendInput, error) {
			return backendInput{Raw: in.Query}, nil
		},
		func(out backendOutput) (domainOutput, error) {
			return domainOutput{Result: out.Data}, nil
		},
	)

	result, err := adapted.Execute(context.Background(), domainInput{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != "test" {
		t.Fatalf("expected 'test', got %q", result.Result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
