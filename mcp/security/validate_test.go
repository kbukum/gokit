package security

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

func TestResultSizeBytes(t *testing.T) {
	t.Parallel()
	if got := ResultSizeBytes(nil); got != 0 {
		t.Errorf("nil result size: got %d want 0", got)
	}
	if got := ResultSizeBytes(&tool.Result{Output: json.RawMessage(`{"a":1}`)}); got != 7 {
		t.Errorf("output size: got %d want 7", got)
	}
	if got := ResultSizeBytes(&tool.Result{Content: "hello"}); got != 5 {
		t.Errorf("content size: got %d want 5", got)
	}
}

func TestFirstValidationError(t *testing.T) {
	t.Parallel()
	if got := FirstValidationError(nil); got != "validation failed" {
		t.Errorf("empty errors fallback: got %q", got)
	}
	errs := []schema.ValidationError{{Path: "a", Message: "required"}}
	if got := FirstValidationError(errs); got != errs[0].Error() {
		t.Errorf("first error: got %q", got)
	}
}

func TestValidateOutput(t *testing.T) {
	t.Parallel()
	// No output schema => always valid.
	if vr := ValidateOutput(tool.Definition{}, &tool.Result{Output: json.RawMessage("garbage")}); !vr.Valid {
		t.Error("missing output schema must skip validation")
	}
	// Error results are not validated.
	def := tool.Definition{OutputSchema: schema.JSON{"type": "object", "required": []any{"sum"}}}
	if vr := ValidateOutput(def, &tool.Result{IsError: true}); !vr.Valid {
		t.Error("error results skip output validation")
	}
	// Valid structured output.
	if vr := ValidateOutput(def, &tool.Result{Output: json.RawMessage(`{"sum":3}`)}); !vr.Valid {
		t.Errorf("valid output rejected: %+v", vr.Errors)
	}
	// Invalid structured output fails closed.
	if vr := ValidateOutput(def, &tool.Result{Output: json.RawMessage(`{"other":1}`)}); vr.Valid {
		t.Error("output missing required field must be invalid")
	}
	// Text-only output (no Output) is validated as a raw string value.
	strDef := tool.Definition{OutputSchema: schema.JSON{"type": "string"}}
	if vr := ValidateOutput(strDef, &tool.Result{Content: "hello"}); !vr.Valid {
		t.Errorf("valid text output rejected: %+v", vr.Errors)
	}
}
