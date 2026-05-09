package authz

import "context"

const (
	ActionSkillActivate = "skill:activate"
	ActionSkillInvoke   = "skill:invoke"
	ActionToolInvoke    = "tool:invoke"
	ActionResourceRead  = "resource:read"
)

type Decider interface {
	Decide(context.Context, Request) (Decision, error)
}

type DeciderFunc func(context.Context, Request) (Decision, error)

func (f DeciderFunc) Decide(ctx context.Context, req Request) (Decision, error) { return f(ctx, req) }

func (e *Engine) Decide(_ context.Context, req Request) (Decision, error) {
	return e.Authorize(req), nil
}

type ApprovalRequest struct {
	Source   string
	Action   string
	Resource Resource
	Reason   string
	Metadata Attributes
}
type ApprovalDecision struct {
	Approved bool
	Reason   string
}
type ApprovalRequester interface {
	RequestApproval(context.Context, ApprovalRequest) (ApprovalDecision, error)
}
