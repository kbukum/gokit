package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func (s *memoryStore) LookupBatch(ctx context.Context, refs []string) (map[string]Record, error) {
	if err := ValidateReferences(refs); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.fail != nil {
		return nil, s.fail
	}
	out := make(map[string]Record, len(refs))
	for _, ref := range refs {
		if row, ok := s.rows[ref]; ok {
			row.Principal = row.Principal.Clone()
			out[ref] = row
		}
	}
	return out, nil
}

func (s *blockingStore) LookupBatch(ctx context.Context, refs []string) (map[string]Record, error) {
	if _, err := s.Lookup(ctx, refs[0]); err != nil {
		return nil, err
	}
	return s.memoryStore.LookupBatch(ctx, refs)
}

func (s *stalledStore) LookupBatch(ctx context.Context, refs []string) (map[string]Record, error) {
	if _, err := s.Lookup(ctx, refs[0]); err != nil {
		return nil, err
	}
	return s.memoryStore.LookupBatch(ctx, refs)
}

func TestValidateReferences(t *testing.T) {
	t.Parallel()
	for _, refs := range [][]string{nil, {""}, {"a", "a"}, {strings.Repeat("x", 513)}, make([]string, MaxLookupBatch+1)} {
		if err := ValidateReferences(refs); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("invalid batch accepted: size %d", len(refs))
		}
	}
}

func TestBatchOutageIsNotAbsence(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record), fail: errors.New("outage")}
	if rows, err := s.LookupBatch(context.Background(), []string{"missing"}); err == nil || rows != nil {
		t.Fatal("outage became authoritative absence")
	}
}
