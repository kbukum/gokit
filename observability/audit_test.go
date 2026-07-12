package observability

import (
	"context"
	"testing"
)

func TestAuditorFunc(t *testing.T) {
	var got AuditEvent
	var auditor Auditor = AuditorFunc(func(_ context.Context, event AuditEvent) {
		got = event
	})

	auditor.Audit(context.Background(), AuditEvent{
		Name:       "login",
		Attributes: map[string]string{"user": "1"},
	})

	if got.Name != "login" {
		t.Fatalf("expected audit event name login, got %q", got.Name)
	}
	if got.Attributes["user"] != "1" {
		t.Fatalf("expected audit attribute user=1, got %q", got.Attributes["user"])
	}
}
