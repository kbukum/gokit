package bench

import (
	"context"
	"errors"
	"testing"
)

func TestEvaluatorFuncWrapping(t *testing.T) {
	t.Parallel()

	eval := EvaluatorFunc("test-eval", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{
			Label: "positive",
			Score: 0.95,
		}, nil
	})

	if eval.Name() != "test-eval" {
		t.Errorf("Name() = %q, want %q", eval.Name(), "test-eval")
	}
	if !eval.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}

	pred, err := eval.Execute(context.Background(), []byte("input"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if pred.Label != "positive" {
		t.Errorf("Label = %q, want %q", pred.Label, "positive")
	}
	if pred.Score != 0.95 {
		t.Errorf("Score = %f, want 0.95", pred.Score)
	}
}

func TestEvaluatorFuncError(t *testing.T) {
	t.Parallel()

	errExpected := errors.New("model unavailable")
	eval := EvaluatorFunc("failing-eval", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{}, errExpected
	})

	_, err := eval.Execute(context.Background(), []byte("input"))
	if !errors.Is(err, errExpected) {
		t.Errorf("expected error %v, got %v", errExpected, err)
	}
}

func TestEvaluatorFuncUsesInput(t *testing.T) {
	t.Parallel()

	eval := EvaluatorFunc("echo-eval", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{
			Label: string(input),
			Score: 1.0,
		}, nil
	})

	pred, err := eval.Execute(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if pred.Label != "hello" {
		t.Errorf("Label = %q, want %q", pred.Label, "hello")
	}
}

func TestEvaluatorInterface(t *testing.T) {
	t.Parallel()

	// Ensure EvaluatorFunc returns something that satisfies Evaluator[string].
	var _ Evaluator[string] = EvaluatorFunc("test", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{}, nil
	})
}
