package bench

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/process"
)

func TestFromProcessName(t *testing.T) {
	t.Parallel()

	eval := FromProcess[string](
		"echo-eval",
		func(s Sample[string]) process.Command {
			return process.Command{Binary: "echo", Args: []string{"hello"}}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{Label: strings.TrimSpace(string(r.Stdout))}, nil
		},
	)

	if eval.Name() != "echo-eval" {
		t.Errorf("Name() = %q, want %q", eval.Name(), "echo-eval")
	}
}

func TestFromProcessIsAvailable(t *testing.T) {
	t.Parallel()

	eval := FromProcess[string](
		"test",
		func(s Sample[string]) process.Command {
			return process.Command{Binary: "echo"}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{}, nil
		},
	)

	if !eval.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}
}

func TestFromProcessExecute(t *testing.T) {
	t.Parallel()

	eval := FromProcess[string](
		"echo-eval",
		func(s Sample[string]) process.Command {
			return process.Command{
				Binary: "echo",
				Args:   []string{"-n", string(s.Input)},
			}
		},
		func(r *process.Result) (Prediction[string], error) {
			label := strings.TrimSpace(string(r.Stdout))
			return Prediction[string]{Label: label, Score: 1.0}, nil
		},
	)

	pred, err := eval.Execute(context.Background(), []byte("positive"))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if pred.Label != "positive" {
		t.Errorf("Label = %q, want %q", pred.Label, "positive")
	}
	if pred.Score != 1.0 {
		t.Errorf("Score = %f, want 1.0", pred.Score)
	}
}

func TestFromProcessErrorBadBinary(t *testing.T) {
	t.Parallel()

	eval := FromProcess[string](
		"bad-eval",
		func(s Sample[string]) process.Command {
			return process.Command{Binary: "/nonexistent/binary/that/does/not/exist"}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{}, nil
		},
	)

	_, err := eval.Execute(context.Background(), []byte("input"))
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "bad-eval") {
		t.Errorf("error = %q, want mention of evaluator name", err.Error())
	}
}

func TestFromProcessPassesInputToCommand(t *testing.T) {
	t.Parallel()

	var captured []byte
	eval := FromProcess[string](
		"capture",
		func(s Sample[string]) process.Command {
			captured = s.Input
			return process.Command{Binary: "echo", Args: []string{"-n", "ok"}}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{Label: "ok"}, nil
		},
	)

	input := []byte("test-input-data")
	_, err := eval.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !bytes.Equal(captured, input) {
		t.Errorf("captured input = %q, want %q", captured, input)
	}
}

func TestFromProcessParseError(t *testing.T) {
	t.Parallel()

	eval := FromProcess[string](
		"parse-fail",
		func(s Sample[string]) process.Command {
			return process.Command{Binary: "echo", Args: []string{"-n", "output"}}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{}, context.DeadlineExceeded
		},
	)

	_, err := eval.Execute(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("expected error from parseOutput")
	}
	if !strings.Contains(err.Error(), "parse output") {
		t.Errorf("error = %q, want mention of 'parse output'", err.Error())
	}
}

func TestFromProcessInterface(t *testing.T) {
	t.Parallel()

	// Ensure FromProcess returns something that satisfies Evaluator[string].
	var _ = FromProcess(
		"test",
		func(s Sample[string]) process.Command {
			return process.Command{Binary: "echo"}
		},
		func(r *process.Result) (Prediction[string], error) {
			return Prediction[string]{}, nil
		},
	)
}
