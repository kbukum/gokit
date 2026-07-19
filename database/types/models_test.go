package types

import (
	"testing"

	"github.com/google/uuid"
)

func TestBaseModelBeforeCreateGeneratesMissingID(t *testing.T) {
	t.Parallel()
	var model BaseModel
	if err := model.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if model.ID == uuid.Nil {
		t.Fatal("expected generated UUID")
	}
}

func TestBaseModelBeforeCreatePreservesExistingID(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	model := BaseModel{ID: id}
	if err := model.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if model.ID != id {
		t.Fatalf("ID = %s, want %s", model.ID, id)
	}
}
