package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/schema"
)

// Call invokes a tool by name with raw JSON input.
//
// Dispatch order:
// schema validation → authz → sensitivity / destructive gate → (if RequireApproval) human approval → invoke (D10).
// Invalid input fails closed with ErrInvalidToolInput before any side effect; any deny short-circuits with ErrToolDenied wrapped with the reason. Per-tool resilience policy is applied by callers via PolicyFor.
func (r *Registry) Call(ctx *Context, name string, input json.RawMessage) (*Result, error) {
	spanCtx, span := observability.StartNamedSpan(ctx.Context, "github.com/kbukum/gokit/tool", "tool.call",
		observability.WithSpanKind(observability.SpanKindInternal),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpToolCall),
			observability.StringAttribute(semconv.GenAIToolName, name),
			observability.StringAttribute("tool.use_id", ctx.ToolUseID),
		),
	)
	defer span.End()
	innerCtx := *ctx
	innerCtx.Context = spanCtx
	ctx = &innerCtx

	t, ok := r.Get(name)
	if !ok {
		err := fmt.Errorf("tool %q not found", name)
		span.RecordError(err)
		return nil, err
	}

	def := t.Definition()
	call := ToolCall{Name: name, ToolUseID: ctx.ToolUseID, Input: input, Definition: def}

	if result := t.Validate(input); !result.Valid {
		err := fmt.Errorf("%w: %s", ErrInvalidToolInput, validationMessage(result))
		span.RecordError(err)
		return nil, err
	}

	r.mu.RLock()
	authorizer := r.authorizer
	evaluator := r.evaluator
	approval := r.approval
	r.mu.RUnlock()

	if authorizer != nil {
		allowed, reason, err := authorizer.Authorize(ctx.Context, call)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("tool %q: authorize: %w", name, err)
		}
		if !allowed {
			err := fmt.Errorf("%w: %s", ErrToolDenied, reason)
			span.RecordError(err)
			return nil, err
		}
	}

	approved := false
	if evaluator != nil {
		for _, predicate := range def.Envelope.SensitiveInvocations {
			decision, reason, err := evaluator.Evaluate(ctx.Context, call, predicate)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("tool %q: evaluate: %w", name, err)
			}
			switch decision {
			case DecisionAllow:
				continue
			case DecisionDeny:
				err := fmt.Errorf("%w: %s", ErrToolDenied, reason)
				span.RecordError(err)
				return nil, err
			case DecisionRequireApproval:
				if !approved {
					if err := r.requireApproval(ctx.Context, approval, call, reason); err != nil {
						span.RecordError(err)
						return nil, err
					}
					approved = true
				}
			default:
				err := fmt.Errorf("tool %q: unknown evaluator decision %q", name, decision)
				span.RecordError(err)
				return nil, err
			}
		}
	}

	// Destructive tools are always human-gated: an irreversible mutation must be approved out of band. With the default DenyHumanApproval this fails closed until an operator wires a real approver. A prior sensitivity predicate may already have obtained approval for this dispatch; approve only once per call.
	if def.Envelope.Safety == SafetyDestructive && !approved {
		if err := r.requireApproval(ctx.Context, approval, call, "destructive tool requires human approval"); err != nil {
			span.RecordError(err)
			return nil, err
		}
	}

	res, err := t.Call(ctx, input)
	if err != nil {
		span.RecordError(err)
	} else {
		r.lifecycle.Touch()
	}
	return res, err
}

// requireApproval routes a call through the human approver, failing closed when no approver is configured or approval is rejected.
func (r *Registry) requireApproval(ctx context.Context, approval HumanApproval, call ToolCall, reason string) error {
	if approval == nil {
		return fmt.Errorf("%w: %s (no approver configured)", ErrToolDenied, reason)
	}
	approved, err := approval.Approve(ctx, call)
	if err != nil {
		return fmt.Errorf("tool %q: approval: %w", call.Name, err)
	}
	if !approved {
		return fmt.Errorf("%w: %s (human approval rejected)", ErrToolDenied, reason)
	}
	return nil
}

// validationMessage renders a compact, human-readable summary of schema validation failures for error text.
func validationMessage(result schema.ValidationResult) string {
	if len(result.Errors) == 0 {
		return "input does not satisfy schema"
	}
	msgs := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}
