package database

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/session"
	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/cleanup"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

type family struct {
	ID               string `gorm:"primaryKey"`
	CurrentReference string
	Generation       uint64
	Subject          string
	Kind             auth.Kind
	Restrictions     string
	ExpiresAt        time.Time
	RetainUntil      time.Time
	AuthenticatedAt  *time.Time
	Revoked          bool
}

func (family) TableName() string { return "auth_session_families" }

type generation struct {
	Reference   string `gorm:"primaryKey"`
	Family      string
	Generation  uint64
	RetainUntil time.Time
}

func (generation) TableName() string { return "auth_session_generations" }

type store struct {
	db    *dbkit.DB
	clock util.Clock
}

// NewStore borrows the injected production database; the caller owns migrations and pool shutdown.
func NewStore(db *dbkit.DB, clocks ...util.Clock) (session.Store, error) {
	if db == nil || db.GormDB == nil {
		return nil, apperrors.InvalidInput("session", "Database is required")
	}
	var clock util.Clock = util.SystemClock{}
	if len(clocks) > 1 {
		return nil, apperrors.InvalidInput("session", "Only one clock may be injected")
	}
	if len(clocks) == 1 {
		if util.IsNil(clocks[0]) {
			return nil, apperrors.InvalidInput("session", "Clock is required")
		}
		clock = clocks[0]
	}
	return &store{db: db, clock: clock}, nil
}

func invalid() error { return auth.Failure("SESSION_INVALID") }

func validate(row session.Record) error {
	if row.Reference == "" || row.Family == "" || row.Generation == 0 || row.Generation >= 1<<63 ||
		row.ExpiresAt.IsZero() || row.RetainUntil.Before(row.ExpiresAt.Add(session.Retention)) ||
		row.Principal.Reference != row.Reference || row.Principal.Credential != auth.Session || !row.Active || row.Revoked {
		return invalid()
	}
	return row.Principal.Validate()
}

// authenticated reports whether a record that starts or relogs a family carries a known, past authentication time.
func authenticated(row session.Record, now time.Time) bool {
	return !row.AuthenticatedAt.IsZero() && !row.AuthenticatedAt.After(now)
}

func (s *store) Create(ctx context.Context, row session.Record) error {
	if err := validate(row); err != nil {
		return err
	}
	now := s.clock.Now()
	if row.Generation != 1 || !now.Before(row.ExpiresAt) || !authenticated(row, now) {
		return invalid()
	}
	encoded, err := json.Marshal(row.Principal.Restrictions)
	if err != nil {
		return err
	}
	return s.db.WithTransaction(ctx, func(tx *gorm.DB) error {
		f := family{ID: row.Family, CurrentReference: row.Reference, Generation: row.Generation, Subject: row.Principal.Subject, Kind: row.Principal.Kind, Restrictions: string(encoded), ExpiresAt: row.ExpiresAt, RetainUntil: row.RetainUntil, AuthenticatedAt: &row.AuthenticatedAt}
		if err := tx.Create(&f).Error; err != nil {
			return err
		}
		g := generation{Reference: row.Reference, Family: row.Family, Generation: row.Generation, RetainUntil: row.RetainUntil}
		return tx.Create(&g).Error
	})
}

type lookupResult struct {
	ID                   string
	CurrentReference     string
	Generation           uint64
	Subject              string
	Kind                 auth.Kind
	Restrictions         string
	ExpiresAt            time.Time
	RetainUntil          time.Time
	AuthenticatedAt      *time.Time
	Revoked              bool
	Reference            string
	CredentialGeneration uint64
}

func (s *store) Lookup(ctx context.Context, ref string) (session.Record, error) {
	rows, err := s.LookupBatch(ctx, []string{ref})
	if err != nil {
		return session.Record{}, err
	}
	if row, ok := rows[ref]; ok {
		return row, nil
	}
	return session.Record{}, apperrors.New(apperrors.ErrCodeNotFound, "Session not found")
}

func (s *store) LookupBatch(ctx context.Context, refs []string) (map[string]session.Record, error) {
	if err := session.ValidateReferences(refs); err != nil {
		return nil, err
	}
	var results []lookupResult
	query := s.db.WithContext(ctx).Table("auth_session_generations AS g").
		Select("f.*, g.reference, g.generation AS credential_generation").
		Joins("JOIN auth_session_families AS f ON f.id = g.family").
		Where("g.reference IN ?", refs).Find(&results)
	if query.Error != nil {
		return nil, query.Error
	}
	rows := make(map[string]session.Record, len(results))
	for i := range results {
		out := &results[i]
		row, err := out.record()
		if err != nil {
			return nil, err
		}
		rows[out.Reference] = row
	}
	return rows, nil
}

func (out *lookupResult) record() (session.Record, error) {
	ref := out.Reference
	var restrictions auth.Restrictions
	if err := json.Unmarshal([]byte(out.Restrictions), &restrictions); err != nil {
		return session.Record{}, err
	}
	p := auth.Principal{Subject: out.Subject, Kind: out.Kind, Credential: auth.Session, Reference: ref, ExpiresAt: out.ExpiresAt, Restrictions: restrictions}
	if err := p.Validate(); err != nil {
		return session.Record{}, err
	}
	row := session.Record{Reference: ref, Family: out.ID, Generation: out.CredentialGeneration, Principal: p, ExpiresAt: out.ExpiresAt, RetainUntil: out.RetainUntil, Active: out.CurrentReference == ref && out.Generation == out.CredentialGeneration, Revoked: out.Revoked}
	if out.AuthenticatedAt != nil {
		row.AuthenticatedAt = *out.AuthenticatedAt
	}
	return row, nil
}

func (s *store) Rotate(ctx context.Context, old string, next session.Record) error {
	return s.replace(ctx, old, next, false)
}

func (s *store) Relogin(ctx context.Context, old string, next session.Record) error {
	return s.replace(ctx, old, next, true)
}

func (s *store) replace(ctx context.Context, old string, next session.Record, relogin bool) error {
	if err := validate(next); err != nil {
		return err
	}
	encoded, err := json.Marshal(next.Principal.Restrictions)
	if err != nil {
		return err
	}
	return s.db.WithTransaction(ctx, func(tx *gorm.DB) error {
		var g generation
		if err := tx.Where("reference = ?", old).Take(&g).Error; err != nil {
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return invalid()
			}
			return err
		}
		if g.Family != next.Family || g.Generation+1 != next.Generation {
			return invalid()
		}
		query := tx.Model(&family{}).Where("id = ? AND current_reference = ? AND generation = ? AND revoked = ?", next.Family, old, g.Generation, false)
		updates := map[string]any{"current_reference": next.Reference, "generation": next.Generation, "restrictions": string(encoded)}
		now := s.clock.Now()
		if relogin {
			if !now.Before(next.ExpiresAt) || next.ExpiresAt.After(now.Add(session.Lifetime)) || !authenticated(next, now) {
				return invalid()
			}
			query = query.Where("(expires_at = ? OR expires_at <= ?)", next.ExpiresAt, now)
			updates["subject"] = next.Principal.Subject
			updates["kind"] = next.Principal.Kind
			updates["expires_at"] = next.ExpiresAt
			updates["retain_until"] = next.RetainUntil
			updates["authenticated_at"] = next.AuthenticatedAt
		} else {
			query = query.Where("expires_at = ? AND expires_at > ? AND retain_until = ? AND subject = ? AND kind = ?", next.ExpiresAt, now, next.RetainUntil, next.Principal.Subject, next.Principal.Kind)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return invalid()
		}
		if relogin {
			if err := tx.Model(&generation{}).Where("family = ?", next.Family).Update("retain_until", next.RetainUntil).Error; err != nil {
				return err
			}
		}
		newGeneration := generation{Reference: next.Reference, Family: next.Family, Generation: next.Generation, RetainUntil: next.RetainUntil}
		return tx.Create(&newGeneration).Error
	})
}

func (s *store) Revoke(ctx context.Context, ref string) (string, error) {
	var id string
	err := s.db.WithTransaction(ctx, func(tx *gorm.DB) error {
		var g generation
		if err := tx.Where("reference = ?", ref).Take(&g).Error; err != nil {
			if stderrors.Is(err, gorm.ErrRecordNotFound) {
				return invalid()
			}
			return err
		}
		id = g.Family
		result := tx.Model(&family{}).Where("id = ?", id).Update("revoked", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return invalid()
		}
		return nil
	})
	return id, err
}

func (s *store) RevokeSubject(ctx context.Context, kind auth.Kind, subject string) (int64, error) {
	if subject == "" || kind != auth.User && kind != auth.Service {
		return 0, apperrors.InvalidInput("subject", "Subject and kind are required")
	}
	var count int64
	err := s.db.WithTransaction(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&family{}).Where("subject = ? AND kind = ? AND revoked = ?", subject, kind, false).Update("revoked", true)
		count = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *store) Cleanup(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 256 {
		return 0, apperrors.InvalidInput("cleanup", "Session cleanup batch must be 1..256")
	}
	var count int64
	err := s.db.WithTransaction(ctx, func(tx *gorm.DB) error {
		clock := util.NewFakeClock(before)
		n, err := cleanup.DeleteExpired[generation](ctx, tx, cleanup.Config{ExpiryField: "RetainUntil", BatchSize: limit, Clock: clock})
		if err != nil {
			return err
		}
		count = n
		if count == int64(limit) {
			return nil
		}
		orphans := tx.Where("NOT EXISTS (SELECT 1 FROM auth_session_generations WHERE auth_session_generations.family = auth_session_families.id)")
		n, err = cleanup.DeleteExpired[family](ctx, orphans, cleanup.Config{ExpiryField: "RetainUntil", BatchSize: limit - int(count), Clock: clock})
		count += n
		return err
	})
	return count, err
}
