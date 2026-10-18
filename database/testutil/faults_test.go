package testutil

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

type note struct {
	ID   int `gorm:"primaryKey"`
	Text string
}

func faultDB(t *testing.T) (*gorm.DB, *Faults) {
	t.Helper()
	c := NewComponent().WithModels(&note{})
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Stop(t.Context()); err != nil {
			t.Error(err)
		}
	})
	f, err := InjectFaults(c.DB())
	if err != nil {
		t.Fatal(err)
	}
	return c.DB(), f
}

func count(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&note{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInjectFaultsRequiresADatabase(t *testing.T) {
	if f, err := InjectFaults(nil); f != nil || err == nil {
		t.Fatalf("InjectFaults(nil) = %v, %v", f, err)
	}
}

func TestDisarmedFaultsChangeNothing(t *testing.T) {
	db, f := faultDB(t)
	if err := db.Create(&note{Text: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	if f.Fired() || count(t, db) != 1 {
		t.Fatal("a disarmed fault fired")
	}
}

// Every statement kind fails at its turn: create, query, update, delete, raw rows and exec.
func TestFailAtFailsTheNthStatementOfEachKind(t *testing.T) {
	db, f := faultDB(t)
	if err := db.Create(&note{ID: 1, Text: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"query", func() error { var n note; return db.First(&n, 1).Error }},
		{"update", func() error { return db.Model(&note{ID: 1}).Update("text", "c").Error }},
		{"row", func() error { var n int; return db.Raw(`SELECT count(*) FROM notes`).Scan(&n).Error }},
		{"exec", func() error { return db.Exec(`UPDATE notes SET text = 'd'`).Error }},
		{"create", func() error { return db.Create(&note{ID: 2, Text: "b"}).Error }},
		{"delete", func() error { return db.Delete(&note{ID: 1}).Error }},
	} {
		name, run := c.name, c.run
		f.FailAt(2)
		if err := run(); err != nil || f.Fired() {
			t.Fatalf("%s: first statement = %v, fired %v", name, err, f.Fired())
		}
		if err := run(); !errors.Is(err, ErrInjected) || !f.Fired() {
			t.Fatalf("%s: second statement = %v, fired %v", name, err, f.Fired())
		}
		f.FailAt(0)
	}
}

// EveryStatement fails each statement of a transaction in turn; each failure rolls back, then one clean run commits.
func TestEveryStatementSweepsATransaction(t *testing.T) {
	db, f := faultDB(t)
	var failed []int
	n, err := f.EveryStatement(func(statement int) error {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&note{ID: 1, Text: "a"}).Error; err != nil {
				return err
			}
			return tx.Create(&note{ID: 2, Text: "b"}).Error
		})
		if err != nil {
			failed = append(failed, statement)
			if rows := count(t, db); rows != 0 {
				t.Errorf("statement %d left %d rows", statement, rows)
			}
		}
		return err
	})
	if err != nil || n != 2 || len(failed) != 2 || count(t, db) != 2 {
		t.Fatalf("EveryStatement = %d, %v; failed %v", n, err, failed)
	}
}

func TestEveryStatementRefusesASwallowedFault(t *testing.T) {
	db, f := faultDB(t)
	_, err := f.EveryStatement(func(int) error {
		_ = db.Create(&note{Text: "a"}).Error
		return nil
	})
	if err == nil {
		t.Fatal("a swallowed fault passed")
	}
}

func TestEveryStatementReportsACleanRunFailure(t *testing.T) {
	_, f := faultDB(t)
	clean := errors.New("clean run failed")
	if _, err := f.EveryStatement(func(int) error { return clean }); !errors.Is(err, clean) {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryStatementIsBounded(t *testing.T) {
	db, f := faultDB(t)
	_, err := f.EveryStatement(func(statement int) error {
		for range statement {
			if err := db.Exec(`SELECT 1`).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		t.Fatal("an unbounded sweep passed")
	}
}
