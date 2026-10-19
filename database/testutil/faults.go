package testutil

import (
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// ErrInjected is the error a failed statement returns.
var ErrInjected = errors.New("injected database fault")

// MaxSweepStatements bounds the statements one EveryStatement sweep fails.
const MaxSweepStatements = 200

const faultCallback = "gokit:testutil:fault"

// Faults fails a chosen SQL statement run through a gorm database. Use it to prove that every statement's failure is reported and rolled back. Transaction begin and commit are not statements; only gorm create, query, update, delete, row and exec calls count.
type Faults struct {
	mu     sync.Mutex
	target int
	seen   int
	fired  bool
}

// InjectFaults registers the fault callbacks on db. Call it once per database; nothing fails until FailAt arms a statement.
func InjectFaults(db *gorm.DB) (*Faults, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	f := &Faults{}
	cb := db.Callback()
	return f, errors.Join(
		cb.Create().Before("gorm:create").Register(faultCallback, f.check),
		cb.Query().Before("gorm:query").Register(faultCallback, f.check),
		cb.Update().Before("gorm:update").Register(faultCallback, f.check),
		cb.Delete().Before("gorm:delete").Register(faultCallback, f.check),
		cb.Row().Before("gorm:row").Register(faultCallback, f.check),
		cb.Raw().Before("gorm:raw").Register(faultCallback, f.check),
	)
}

// FailAt fails the nth statement counted from now, starting at 1. Zero disarms.
func (f *Faults) FailAt(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.target, f.seen, f.fired = max(n, 0), 0, false
}

// Fired reports whether the armed statement failed.
func (f *Faults) Fired() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fired
}

func (f *Faults) check(db *gorm.DB) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.target == 0 || f.fired {
		return
	}
	f.seen++
	if f.seen == f.target {
		f.fired = true
		_ = db.AddError(ErrInjected)
	}
}

// EveryStatement runs op once per statement it executes, failing that statement each time, then once with no fault. op receives the failed statement's number, or the clean run's, and must leave state as it found it whenever it fails. It returns how many statements were failed. A run that hides its failed statement, a failing clean run and more than [MaxSweepStatements] statements are errors.
func (f *Faults) EveryStatement(op func(statement int) error) (int, error) {
	defer f.FailAt(0)
	for n := 1; n <= MaxSweepStatements+1; n++ {
		f.FailAt(n)
		err := op(n)
		fired := f.Fired()
		switch {
		case !fired && err != nil:
			return n - 1, fmt.Errorf("clean run after %d statements failed: %w", n-1, err)
		case !fired:
			return n - 1, nil
		case err == nil:
			return n, fmt.Errorf("statement %d failed but the operation succeeded", n)
		}
	}
	return MaxSweepStatements, fmt.Errorf("operation ran more than %d statements", MaxSweepStatements)
}
