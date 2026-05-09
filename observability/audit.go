package observability

import "context"

type AuditEvent struct {
	Name       string
	Attributes map[string]string
}
type Auditor interface {
	Audit(context.Context, AuditEvent)
}
type AuditorFunc func(context.Context, AuditEvent)

func (f AuditorFunc) Audit(ctx context.Context, event AuditEvent) { f(ctx, event) }
