package query

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type cursorNamedInt int32

type cursorCustomString string

func (cursorCustomString) Value() (driver.Value, error) { return "encoded", nil }

type cursorGormString string

func (v cursorGormString) GormValue(context.Context, *gorm.DB) clause.Expr {
	return clause.Expr{SQL: "LOWER(?)", Vars: []any{string(v)}}
}

type cursorGormInt int

func (v *cursorGormInt) GormValue(context.Context, *gorm.DB) clause.Expr {
	return clause.Expr{SQL: "?", Vars: []any{int(*v) + 1}}
}

type cursorFieldModel struct {
	ID         int `gorm:"primaryKey"`
	Integer    cursorNamedInt
	Unsigned   uint64
	Text       string
	Time       time.Time
	UUID       uuid.UUID
	NullInt    sql.NullInt64  `gorm:"not null"`
	NullString sql.NullString `gorm:"not null"`
	NullTime   sql.NullTime   `gorm:"not null"`
	Custom     cursorCustomString
	GormString cursorGormString
	GormInt    cursorGormInt
	Serialized string `gorm:"serializer:json"`
	Pointer    *int
	Float      float64
}

func TestCursorFieldRepresentations(t *testing.T) {
	t.Parallel()
	model, err := schema.Parse(&cursorFieldModel{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ID", "Integer", "Unsigned", "Text", "Time", "UUID"} {
		if !cursorField(model.LookUpField(name)) {
			t.Errorf("supported field %s rejected", name)
		}
	}
	for _, name := range []string{"NullInt", "NullString", "NullTime", "Custom", "GormString", "GormInt", "Serialized", "Pointer", "Float", "Missing"} {
		if cursorField(model.LookUpField(name)) {
			t.Errorf("unsupported field %s accepted", name)
		}
	}
}
