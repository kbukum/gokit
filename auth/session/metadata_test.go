package session

import (
	"context"
	"testing"
)

func TestInvalidStoreMetadataFailsClosed(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	row := s.rows[issued.Principal.Reference]
	row.Family = ""
	s.rows[issued.Principal.Reference] = row
	s.mu.Unlock()
	if _, err := m.Authenticate(request(issued.Token)); err == nil {
		t.Fatal("accepted incomplete family")
	}
}
