package apikey

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func TestRotateKeyGraceBoundaries(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		grace     time.Duration
		expires   *time.Time
		wantGrace time.Time
	}{
		"zero grace ends the old key now": {0, nil, epoch},
		"positive grace":                  {time.Minute, nil, epoch.Add(time.Minute)},
		"grace never outlives expiry":     {time.Hour, at(epoch.Add(time.Minute)), epoch.Add(time.Minute)},
		"maximum grace":                   {MaxGrace, nil, epoch.Add(MaxGrace)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clock := util.NewFakeClock(epoch)
			m := newTestManager(t, nil, clock)
			old := issue(t, m, "old", tc.expires)
			result, err := m.RotateKey(context.Background(), "old", RotateRequest{NewKeyID: "new", Grace: tc.grace})
			if err != nil {
				t.Fatalf("RotateKey: %v", err)
			}
			if !result.GraceEndsAt.Equal(tc.wantGrace) || result.Record.ID != "new" || result.Record.OwnerID != "owner" || result.Record.Scopes[0] != "read" {
				t.Fatalf("result = %+v", result)
			}
			if _, err := m.ValidateKey(context.Background(), result.Issued.PlainKey, "read"); err != nil {
				t.Fatalf("replacement: %v", err)
			}
			if tc.wantGrace.After(epoch) {
				clock.Set(tc.wantGrace.Add(-time.Nanosecond))
				if _, err := m.ValidateKey(context.Background(), old.PlainKey); err != nil {
					t.Fatalf("old key inside grace: %v", err)
				}
			}
			clock.Set(tc.wantGrace)
			if _, err := m.ValidateKey(context.Background(), old.PlainKey); reason(err) != "INVALID_CREDENTIAL" {
				t.Fatalf("old key at grace end: %v", err)
			}
			if _, err := m.ValidateKeyID(context.Background(), "old"); reason(err) != "INVALID_CREDENTIAL" {
				t.Fatalf("old key id at grace end: %v", err)
			}
		})
	}
}

func TestRotateKeyRejectsInvalidRequests(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, nil, nil)
	issue(t, m, "old", nil)
	for name, req := range map[string]RotateRequest{
		"missing new id":  {},
		"negative grace":  {NewKeyID: "new", Grace: -time.Nanosecond},
		"excessive grace": {NewKeyID: "new", Grace: MaxGrace + time.Nanosecond},
		"past expiry":     {NewKeyID: "new", ExpiresAt: at(epoch)},
	} {
		if _, err := m.RotateKey(context.Background(), "old", req); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := m.RotateKey(context.Background(), "absent", RotateRequest{NewKeyID: "new"}); apperrors.Normalize(err).Code != apperrors.ErrCodeNotFound {
		t.Fatalf("missing old key: %v", err)
	}
}

func TestRotateKeyIsSingleWinnerAndPropagatesStoreErrors(t *testing.T) {
	t.Parallel()
	store := &faultStore{Store: newMemory(t)}
	m := newTestManager(t, store, nil)
	issue(t, m, "old", nil)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"a", "b"} {
		wg.Go(func() {
			_, errs[i] = m.RotateKey(context.Background(), "old", RotateRequest{NewKeyID: id, Grace: time.Minute})
		})
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("want exactly one winner: %v", errs)
	}
	issue(t, m, "other", nil)
	store.rotateErr = errors.New("rotate failed")
	if _, err := m.RotateKey(context.Background(), "other", RotateRequest{NewKeyID: "c"}); !errors.Is(err, store.rotateErr) {
		t.Fatalf("store failure: %v", err)
	}
	store.getErr = errors.New("read failed")
	if _, err := m.RotateKey(context.Background(), "other", RotateRequest{NewKeyID: "c"}); !errors.Is(err, store.getErr) {
		t.Fatalf("read failure: %v", err)
	}
}
