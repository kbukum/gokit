package session

import (
	"context"
	"slices"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/lease"
	apperrors "github.com/kbukum/gokit/errors"
)

const (
	watchLease    = 3 * time.Second
	watchInterval = time.Second
)

// watchKey binds a stream to the generation it was admitted with and to its family, so rotation ends it by reference
// and logout from any generation ends it by family.
type watchKey struct {
	reference, family, subject string
	kind                       auth.Kind
}

// Acquire performs a final authoritative lookup under the local revocation gate and leases the stream to it.
// The returned lifetime ends on expiry, logout, rotation, an unrenewed lease or manager shutdown; context.Cause
// reports a [lease] cause. Release is idempotent and must be called by the stream owner.
func (m *Manager) Acquire(ctx context.Context, ref string) (context.Context, func(), error) {
	admission, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	value, err := admitted(admission, m, func() (acquisition, error) { return m.acquire(admission, ctx, ref) })
	return value.lifetime, value.release, err
}

func (m *Manager) acquire(admission, owner context.Context, ref string) (acquisition, error) {
	checked := m.timing.Now()
	row, err := m.lookup(admission, ref)
	if err != nil {
		return acquisition{}, err
	}
	if failure := apperrors.FromContext(admission, "session.acquire"); failure != nil {
		return acquisition{}, failure
	}
	remaining := row.ExpiresAt.Sub(m.clock.Now())
	life, release, err := m.leases.Acquire(owner, watchKey{reference: ref, family: row.Family, subject: row.Principal.Subject, kind: row.Principal.Kind}, lease.Grant{CheckedAt: checked, Lifetime: min(remaining, Lifetime)})
	if err != nil {
		return acquisition{}, err
	}
	return acquisition{lifetime: life, release: release}, nil
}

// Nudge requests early revalidation through the owned renewal worker. Requests coalesce.
func (m *Manager) Nudge() { m.leases.Nudge() }

// checkWatches confirms keys whose generation is still active in the same family and denies keys whose session is
// invalid. A reference whose lookup fails for another reason is left unanswered, so its lease runs out.
func (m *Manager) checkWatches(ctx context.Context, keys []watchKey) (map[watchKey]bool, error) {
	byReference := make(map[string][]watchKey, len(keys))
	for _, key := range keys {
		byReference[key.reference] = append(byReference[key.reference], key)
	}
	answers := make(map[watchKey]bool, len(keys))
	refs := make([]string, 0, len(byReference))
	for ref := range byReference {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	ctx, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	for start := 0; start < len(refs); start += MaxLookupBatch {
		chunk := refs[start:min(start+MaxLookupBatch, len(refs))]
		rows, err := m.store.LookupBatch(ctx, chunk)
		if err != nil {
			return answers, storeFailure(ctx, err)
		}
		if err := ctx.Err(); err != nil {
			return answers, storeFailure(ctx, err)
		}
		for _, ref := range chunk {
			row, present := rows[ref]
			var validationErr error
			if present {
				row, validationErr = m.validateRecord(ref, row)
			}
			if validationErr != nil && !invalidSession(validationErr) {
				return answers, validationErr
			}
			for _, key := range byReference[ref] {
				answers[key] = present && validationErr == nil && row.Family == key.family && row.Principal.Subject == key.subject && row.Principal.Kind == key.kind
			}
		}
	}
	return answers, nil
}

// invalidSession reports an authoritative answer that the session no longer exists, as opposed to a store failure.
func invalidSession(err error) bool {
	app, ok := apperrors.AsAppError(err)
	return ok && app.Code == apperrors.ErrCodeUnauthorized
}

func (m *Manager) endReference(ref string) {
	m.leases.Revoke(func(key watchKey) bool { return key.reference == ref })
}

func (m *Manager) endFamily(family string) {
	m.leases.Revoke(func(key watchKey) bool { return key.family == family })
}

// Cleanup performs exactly one indexed batch of at most 256 retained generations.
func (m *Manager) Cleanup(ctx context.Context) (int64, error) {
	if err := m.ready(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	n, err := m.store.Cleanup(ctx, m.clock.Now(), 256)
	if err != nil {
		return 0, storeFailure(ctx, err)
	}
	return n, nil
}

// runCleanup deletes retained generations every ten seconds and reports a failed batch. A failure ends no stream:
// retained generations are already invalid, and streams stay fail-closed through their leases, which run out within
// watchLease when the store cannot answer renewal lookups.
func (m *Manager) runCleanup() {
	defer close(m.done)
	for {
		timer := m.timing.NewTimer(10 * time.Second)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
		if _, err := m.Cleanup(m.ctx); err != nil && m.ctx.Err() == nil {
			m.report(m.ctx, err)
		}
	}
}

// Close ends local stream lifetimes synchronously and waits for all owned workers.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	leaseErr := m.leases.Close(ctx)
	m.stop()
	select {
	case <-m.done:
		return leaseErr
	case <-ctx.Done():
		return apperrors.FromContext(ctx, "session.close")
	}
}
