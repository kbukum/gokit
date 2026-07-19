package memory_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kbukum/gokit/agent/memory"
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

func TestMapStore_LoadEmpty(t *testing.T) {
	store := memory.NewMapStore()
	msgs, err := store.Load(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil, got %v", msgs)
	}
}

func TestMapStore_SaveAndLoad(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	original := []chat.Message{
		chat.User("hello"),
		chat.Assistant("hi there"),
	}

	if err := store.Save(ctx, "s1", original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}

	if u, ok := loaded[0].(chat.UserMessage); !ok {
		t.Errorf("msg[0] type = %T, want UserMessage", loaded[0])
	} else if ai.TextOf(u.Content) != "hello" {
		t.Errorf("msg[0] text = %q, want %q", ai.TextOf(u.Content), "hello")
	}

	if a, ok := loaded[1].(chat.AssistantMessage); !ok {
		t.Errorf("msg[1] type = %T, want AssistantMessage", loaded[1])
	} else if a.Text() != "hi there" {
		t.Errorf("msg[1] text = %q, want %q", a.Text(), "hi there")
	}
}

func TestMapStore_DeepCopy(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	original := []chat.Message{chat.User("original")}
	if err := store.Save(ctx, "s1", original); err != nil {
		t.Fatal(err)
	}

	original[0] = chat.User("mutated")

	loaded, _ := store.Load(ctx, "s1")
	if u, ok := loaded[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "original" {
		t.Error("stored message was aliased with original")
	}

	loaded[0] = chat.User("also mutated")

	loaded2, _ := store.Load(ctx, "s1")
	if u, ok := loaded2[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "original" {
		t.Error("stored message was aliased with loaded result")
	}
}

func TestMapStore_Append(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	if err := store.Save(ctx, "s1", []chat.Message{chat.User("one")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "s1", chat.Assistant("two"), chat.User("three")); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load(ctx, "s1")
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}
}

func TestMapStore_Clear(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	_ = store.Save(ctx, "s1", []chat.Message{chat.User("data")})
	if err := store.Clear(ctx, "s1"); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load(ctx, "s1")
	if loaded != nil {
		t.Errorf("expected nil after clear, got %v", loaded)
	}
}

func TestMapStore_ConcurrentAccess(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			sid := fmt.Sprintf("session-%d", id%5)
			msg := chat.User(fmt.Sprintf("msg-%d", id))
			_ = store.Append(ctx, sid, msg)
			_, _ = store.Load(ctx, sid)
		}(i)
	}

	wg.Wait()

	for i := 0; i < 5; i++ {
		loaded, err := store.Load(ctx, fmt.Sprintf("session-%d", i))
		if err != nil {
			t.Fatalf("error loading session-%d: %v", i, err)
		}
		if len(loaded) == 0 {
			t.Errorf("session-%d has no messages", i)
		}
	}
}

func TestSlidingWindowStore_WindowEnforcement(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	msgs := []chat.Message{
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
		chat.User("5"),
	}
	_ = store.Save(ctx, "s1", msgs)

	sw := memory.NewSlidingWindowStore(store, 3)

	loaded, err := sw.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}

	if u, ok := loaded[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "3" {
		t.Errorf("msg[0] = %T/%q, want UserMessage '3'", loaded[0], loaded[0])
	}
}

func TestSlidingWindowStore_SystemMessagePreserved(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	msgs := []chat.Message{
		chat.System("system prompt"),
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
		chat.User("5"),
	}
	_ = store.Save(ctx, "s1", msgs)

	sw := memory.NewSlidingWindowStore(store, 2)

	loaded, err := sw.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}

	if _, ok := loaded[0].(chat.SystemMessage); !ok {
		t.Errorf("first message should be SystemMessage, got %T", loaded[0])
	}
	if u, ok := loaded[2].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "5" {
		t.Errorf("last message should be User '5', got %T", loaded[2])
	}
}

func TestSlidingWindowStore_SaveTrims(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	sw := memory.NewSlidingWindowStore(store, 2)

	msgs := []chat.Message{
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
	}
	_ = sw.Save(ctx, "s1", msgs)

	raw, _ := store.Load(ctx, "s1")
	if len(raw) != 2 {
		t.Fatalf("expected 2 stored messages, got %d", len(raw))
	}
}

func TestSlidingWindowStore_SmallHistory(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	_ = store.Save(ctx, "s1", []chat.Message{chat.User("only one")})

	sw := memory.NewSlidingWindowStore(store, 10)
	loaded, _ := sw.Load(ctx, "s1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded))
	}
}

func TestSlidingWindowStore_Append(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	sw := memory.NewSlidingWindowStore(store, 2)
	_ = sw.Append(ctx, "s1", chat.User("a"), chat.Assistant("b"), chat.User("c"))

	raw, _ := store.Load(ctx, "s1")
	if len(raw) != 3 {
		t.Fatalf("expected 3 stored messages, got %d", len(raw))
	}

	loaded, _ := sw.Load(ctx, "s1")
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages from window load, got %d", len(loaded))
	}
}

func TestSlidingWindowStore_Clear(t *testing.T) {
	store := memory.NewMapStore()
	ctx := context.Background()

	sw := memory.NewSlidingWindowStore(store, 5)
	_ = sw.Save(ctx, "s1", []chat.Message{chat.User("data")})
	_ = sw.Clear(ctx, "s1")

	loaded, _ := sw.Load(ctx, "s1")
	if loaded != nil {
		t.Errorf("expected nil after clear, got %v", loaded)
	}
}
