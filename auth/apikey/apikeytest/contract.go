package apikeytest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/apikey"
	apperrors "github.com/kbukum/gokit/errors"
)

// Harness supplies the store under test.
type Harness struct {
	// NewStore returns an empty store with room for at least 64 keys. Each call must be independent.
	NewStore func(t *testing.T) apikey.Store
	// OwnerID is the owner recorded on every key; it defaults to "owner". Stores that require an existing owner
	// create it in NewStore.
	OwnerID string
}

// base is microsecond-precise so stores backed by SQL timestamps round-trip exactly.
var base = time.Date(2030, 1, 2, 3, 4, 5, 6000, time.UTC)

// Run executes the store contract as parallel subtests.
func Run(t *testing.T, h Harness) {
	t.Helper()
	if h.NewStore == nil {
		t.Fatal("apikeytest: Harness.NewStore is required")
	}
	if h.OwnerID == "" {
		h.OwnerID = "owner"
	}
	tests := map[string]func(*testing.T, *fixture){
		"CreateAndRead":                    testCreateAndRead,
		"CreateRejectsDuplicates":          testCreateRejectsDuplicates,
		"MissingKeysAreNotFound":           testMissingKeysAreNotFound,
		"RevokeIsOneWayAndIdempotent":      testRevokeIsOneWay,
		"RotateMovesOldKeyIntoGrace":       testRotate,
		"RotateComparesAndSwaps":           testRotateComparesAndSwaps,
		"RotateIsAtomic":                   testRotateIsAtomic,
		"ConcurrentRotationsHaveOneWinner": testConcurrentRotations,
		"RevokeRacingRotation":             testRevokeRacingRotation,
		"CanceledContextChangesNothing":    testCanceled,
		"DeleteReleasesDigest":             testDelete,
		"UpdateLastUsed":                   testUpdateLastUsed,
		"InvalidRotationChangesNothing":    testInvalidRotation,
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			test(t, &fixture{store: h.NewStore(t), owner: h.OwnerID})
		})
	}
}

func testInvalidRotation(t *testing.T, f *fixture) {
	old := f.create(t, "old")
	for name, mutate := range map[string]func(*apikey.Rotation){
		"missing check time": func(r *apikey.Rotation) { r.At = time.Time{} },
		"grace before check": func(r *apikey.Rotation) { r.GraceEndsAt = r.At.Add(-time.Second) },
		"unbounded grace":    func(r *apikey.Rotation) { r.GraceEndsAt = r.At.Add(apikey.MaxGrace + time.Second) },
		"nil replacement":    func(r *apikey.Rotation) { r.Replacement = nil },
	} {
		r := rotation("old", f.key("replacement"))
		mutate(&r)
		if err := f.store.Rotate(context.Background(), r); code(err) != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
		same(t, f.get(t, "old"), old)
		f.absent(t, "replacement")
	}
}

type fixture struct {
	store apikey.Store
	owner string
}

// key returns a valid record whose digest is derived from its id.
func (f *fixture) key(id string) *apikey.Key {
	sum := sha256.Sum256([]byte(id))
	expires := base.Add(time.Hour)
	return &apikey.Key{
		ID: id, OwnerID: f.owner, Name: "name-" + id, KeyPrefix: "key", KeyDigest: hex.EncodeToString(sum[:]),
		Scopes: []string{"read"}, Kind: auth.User, RestrictionMode: auth.Restricted, Resources: []string{"resource"},
		ExpiresAt: &expires, CreatedAt: base,
	}
}

func (f *fixture) create(t *testing.T, id string) *apikey.Key {
	t.Helper()
	key := f.key(id)
	if err := f.store.Create(context.Background(), key.Clone()); err != nil {
		t.Fatalf("Create(%s): %v", id, err)
	}
	return key
}

func (f *fixture) get(t *testing.T, id string) *apikey.Key {
	t.Helper()
	key, err := f.store.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID(%s): %v", id, err)
	}
	return key
}

func (f *fixture) absent(t *testing.T, id string) {
	t.Helper()
	if _, err := f.store.GetByID(context.Background(), id); code(err) != apperrors.ErrCodeNotFound {
		t.Fatalf("GetByID(%s) = %v, want NOT_FOUND", id, err)
	}
}

func rotation(old string, replacement *apikey.Key) apikey.Rotation {
	return apikey.Rotation{OldID: old, Replacement: replacement, GraceEndsAt: base.Add(time.Minute), At: base}
}

func code(err error) apperrors.ErrorCode {
	if err == nil {
		return ""
	}
	return apperrors.Normalize(err).Code
}

func same(t *testing.T, got, want *apikey.Key) {
	t.Helper()
	if got == nil || got.ID != want.ID || got.OwnerID != want.OwnerID || got.Name != want.Name ||
		got.KeyPrefix != want.KeyPrefix || got.KeyDigest != want.KeyDigest || got.Kind != want.Kind ||
		got.RestrictionMode != want.RestrictionMode || fmt.Sprint(got.Scopes) != fmt.Sprint(want.Scopes) ||
		fmt.Sprint(got.Resources) != fmt.Sprint(want.Resources) || got.RotatedByID != want.RotatedByID ||
		!sameTime(got.ExpiresAt, want.ExpiresAt) || !sameTime(got.GraceEndsAt, want.GraceEndsAt) ||
		!sameTime(got.RevokedAt, want.RevokedAt) || !sameTime(got.LastUsedAt, want.LastUsedAt) || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("stored key differs:\n got %+v\nwant %+v", got, want)
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func testCreateAndRead(t *testing.T, f *fixture) {
	want := f.create(t, "one")
	byID := f.get(t, "one")
	same(t, byID, want)
	byDigest, err := f.store.GetByDigest(context.Background(), want.KeyDigest)
	if err != nil {
		t.Fatalf("GetByDigest: %v", err)
	}
	same(t, byDigest, want)
	byID.Scopes[0], byID.Resources[0] = "write", "other"
	*byID.ExpiresAt = base
	same(t, f.get(t, "one"), want)
}

func testCreateRejectsDuplicates(t *testing.T, f *fixture) {
	original := f.create(t, "one")
	sameID := f.key("one")
	sameID.KeyDigest = f.key("other").KeyDigest
	sameDigest := f.key("two")
	sameDigest.KeyDigest = original.KeyDigest
	for name, key := range map[string]*apikey.Key{"id": sameID, "digest": sameDigest} {
		if err := f.store.Create(context.Background(), key); code(err) != apperrors.ErrCodeAlreadyExists {
			t.Fatalf("duplicate %s: %v, want ALREADY_EXISTS", name, err)
		}
	}
	same(t, f.get(t, "one"), original)
	f.absent(t, "two")
}

func testMissingKeysAreNotFound(t *testing.T, f *fixture) {
	ctx := context.Background()
	operations := map[string]func() error{
		"GetByID":        func() error { _, err := f.store.GetByID(ctx, "absent"); return err },
		"GetByDigest":    func() error { _, err := f.store.GetByDigest(ctx, f.key("absent").KeyDigest); return err },
		"Revoke":         func() error { return f.store.Revoke(ctx, "absent", base) },
		"Rotate":         func() error { return f.store.Rotate(ctx, rotation("absent", f.key("replacement"))) },
		"UpdateLastUsed": func() error { return f.store.UpdateLastUsed(ctx, "absent", base) },
		"Delete":         func() error { return f.store.Delete(ctx, "absent") },
	}
	for name, operation := range operations {
		if err := operation(); code(err) != apperrors.ErrCodeNotFound {
			t.Fatalf("%s: %v, want NOT_FOUND", name, err)
		}
	}
	f.absent(t, "replacement")
}

func testRevokeIsOneWay(t *testing.T, f *fixture) {
	want := f.create(t, "one")
	first := base.Add(time.Second)
	if err := f.store.Revoke(context.Background(), "one", first); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := f.store.Revoke(context.Background(), "one", first.Add(time.Second)); err != nil {
		t.Fatalf("second Revoke: %v", err)
	}
	want.RevokedAt = &first
	same(t, f.get(t, "one"), want)
}

func testRotate(t *testing.T, f *fixture) {
	old := f.create(t, "old")
	replacement := f.key("new")
	if err := f.store.Rotate(context.Background(), rotation("old", replacement.Clone())); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	grace := base.Add(time.Minute)
	old.GraceEndsAt, old.RotatedByID = &grace, "new"
	same(t, f.get(t, "old"), old)
	got, err := f.store.GetByDigest(context.Background(), replacement.KeyDigest)
	if err != nil {
		t.Fatalf("replacement by digest: %v", err)
	}
	same(t, got, replacement)
}

func testRotateComparesAndSwaps(t *testing.T, f *fixture) {
	ctx := context.Background()
	f.create(t, "rotated")
	if err := f.store.Rotate(ctx, rotation("rotated", f.key("first"))); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	f.create(t, "revoked")
	if err := f.store.Revoke(ctx, "revoked", base); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	expired := f.create(t, "expired")
	cases := map[string]apikey.Rotation{
		"already rotated": rotation("rotated", f.key("second")),
		"revoked":         rotation("revoked", f.key("after-revoke")),
		"expired":         {OldID: "expired", Replacement: f.key("after-expiry"), GraceEndsAt: expired.ExpiresAt.Add(time.Minute), At: *expired.ExpiresAt},
	}
	for name, r := range cases {
		if err := f.store.Rotate(ctx, r); code(err) != apperrors.ErrCodeConflict {
			t.Fatalf("%s: %v, want CONFLICT", name, err)
		}
		f.absent(t, r.Replacement.ID)
	}
	if got := f.get(t, "rotated"); got.RotatedByID != "first" {
		t.Fatalf("second rotation replaced the first: %+v", got)
	}
}

func testRotateIsAtomic(t *testing.T, f *fixture) {
	old := f.create(t, "old")
	existing := f.create(t, "existing")
	duplicateID := f.key("existing")
	duplicateID.KeyDigest = f.key("fresh").KeyDigest
	duplicateDigest := f.key("fresh")
	duplicateDigest.KeyDigest = existing.KeyDigest
	for name, replacement := range map[string]*apikey.Key{"id": duplicateID, "digest": duplicateDigest} {
		if err := f.store.Rotate(context.Background(), rotation("old", replacement)); code(err) != apperrors.ErrCodeAlreadyExists {
			t.Fatalf("replacement with duplicate %s: %v, want ALREADY_EXISTS", name, err)
		}
		same(t, f.get(t, "old"), old)
		same(t, f.get(t, "existing"), existing)
	}
	f.absent(t, "fresh")
}

func testConcurrentRotations(t *testing.T, f *fixture) {
	f.create(t, "old")
	const contenders = 8
	errs := make([]error, contenders)
	var wg sync.WaitGroup
	for i := range contenders {
		wg.Go(func() {
			errs[i] = f.store.Rotate(context.Background(), rotation("old", f.key(fmt.Sprintf("new-%d", i))))
		})
	}
	wg.Wait()
	winner := -1
	for i, err := range errs {
		switch {
		case err == nil && winner >= 0:
			t.Fatalf("rotations %d and %d both won", winner, i)
		case err == nil:
			winner = i
		case code(err) != apperrors.ErrCodeConflict:
			t.Fatalf("losing rotation %d: %v, want CONFLICT", i, err)
		}
	}
	if winner < 0 {
		t.Fatal("no rotation won")
	}
	if got := f.get(t, "old"); got.RotatedByID != fmt.Sprintf("new-%d", winner) {
		t.Fatalf("old key names %q, winner was new-%d", got.RotatedByID, winner)
	}
	for i := range contenders {
		if i != winner {
			f.absent(t, fmt.Sprintf("new-%d", i))
		}
	}
}

// testRevokeRacingRotation proves the two linearizable outcomes: revocation first makes rotation fail without a
// replacement; rotation first leaves a replacement and a revoked old key whose grace no longer applies.
func testRevokeRacingRotation(t *testing.T, f *fixture) {
	for i := range 16 {
		old, replacement := fmt.Sprintf("old-%d", i), fmt.Sprintf("new-%d", i)
		f.create(t, old)
		var rotateErr, revokeErr error
		var wg sync.WaitGroup
		wg.Go(func() { rotateErr = f.store.Rotate(context.Background(), rotation(old, f.key(replacement))) })
		wg.Go(func() { revokeErr = f.store.Revoke(context.Background(), old, base) })
		wg.Wait()
		if revokeErr != nil {
			t.Fatalf("Revoke: %v", revokeErr)
		}
		got := f.get(t, old)
		if got.RevokedAt == nil {
			t.Fatal("revocation lost to rotation")
		}
		switch {
		case rotateErr == nil:
			if got.RotatedByID != replacement {
				t.Fatalf("rotation succeeded but old key names %q", got.RotatedByID)
			}
			f.get(t, replacement)
		case code(rotateErr) == apperrors.ErrCodeConflict:
			if got.RotatedByID != "" {
				t.Fatalf("failed rotation still marked old key: %+v", got)
			}
			f.absent(t, replacement)
		default:
			t.Fatalf("Rotate: %v, want success or CONFLICT", rotateErr)
		}
	}
}

func testCanceled(t *testing.T, f *fixture) {
	want := f.create(t, "one")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operations := map[string]func() error{
		"Create":         func() error { return f.store.Create(ctx, f.key("two")) },
		"GetByID":        func() error { _, err := f.store.GetByID(ctx, "one"); return err },
		"GetByDigest":    func() error { _, err := f.store.GetByDigest(ctx, want.KeyDigest); return err },
		"Revoke":         func() error { return f.store.Revoke(ctx, "one", base) },
		"Rotate":         func() error { return f.store.Rotate(ctx, rotation("one", f.key("three"))) },
		"UpdateLastUsed": func() error { return f.store.UpdateLastUsed(ctx, "one", base) },
		"Delete":         func() error { return f.store.Delete(ctx, "one") },
	}
	for name, operation := range operations {
		if err := operation(); err == nil {
			t.Fatalf("%s succeeded with a canceled context", name)
		}
	}
	same(t, f.get(t, "one"), want)
	f.absent(t, "two")
	f.absent(t, "three")
}

func testDelete(t *testing.T, f *fixture) {
	key := f.create(t, "one")
	if err := f.store.Delete(context.Background(), "one"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.store.GetByDigest(context.Background(), key.KeyDigest); code(err) != apperrors.ErrCodeNotFound {
		t.Fatalf("deleted digest: %v, want NOT_FOUND", err)
	}
	f.create(t, "one")
}

func testUpdateLastUsed(t *testing.T, f *fixture) {
	want := f.create(t, "one")
	used := base.Add(time.Second)
	if err := f.store.UpdateLastUsed(context.Background(), "one", used); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	want.LastUsedAt = &used
	same(t, f.get(t, "one"), want)
}
