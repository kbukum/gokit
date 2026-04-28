package validation

import "testing"

func BenchmarkValidator_HappyPath(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := New().
			Required("name", "alice").
			MaxLength("name", "alice", 64).
			MinLength("name", "alice", 1).
			InRange("age", 30, 0, 120).
			OneOf("role", "admin", []string{"admin", "user", "guest"})
		if err := v.Validate(); err != nil {
			b.Fatalf("validate: %v", err)
		}
	}
}

func BenchmarkValidator_FailFast(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := New().
			Required("name", "").
			MaxLength("name", "", 0).
			InRange("age", 999, 0, 120)
		if err := v.Validate(); err == nil {
			b.Fatal("expected error")
		}
	}
}

func BenchmarkValidator_Pattern(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := New().Pattern("email", "user@example.com", `^[^@]+@[^@]+\.[^@]+$`)
		if err := v.Validate(); err != nil {
			b.Fatalf("validate: %v", err)
		}
	}
}

func BenchmarkValidateUUID(b *testing.B) {
	const id = "550e8400-e29b-41d4-a716-446655440000"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ValidateUUID("id", id); err != nil {
			b.Fatalf("uuid: %v", err)
		}
	}
}

type benchUser struct {
	Name  string `validate:"required,max=64"`
	Email string `validate:"required,email"`
	Age   int    `validate:"gte=0,lte=120"`
}

func BenchmarkValidateStruct(b *testing.B) {
	u := benchUser{Name: "alice", Email: "alice@example.com", Age: 30}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Validate(u); err != nil {
			b.Fatalf("validate: %v", err)
		}
	}
}
