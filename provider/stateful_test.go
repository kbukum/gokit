package provider_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

// --- Test types for Stateful ---

type chatRequest struct {
	SessionID string
	Message   string
}

type chatResponse struct {
	Reply   string
	History []string
}

type chatState struct {
	Messages []string
}

type chatProvider struct{}

func (p *chatProvider) Name() string                       { return "chat" }
func (p *chatProvider) IsAvailable(_ context.Context) bool { return true }

func (p *chatProvider) Execute(_ context.Context, req chatRequest) (chatResponse, error) {
	return chatResponse{
		Reply:   "echo:" + req.Message,
		History: []string{req.Message},
	}, nil
}

var _ provider.RequestResponse[chatRequest, chatResponse] = (*chatProvider)(nil)

// --- Tests ---

func TestStateful_BasicFlow(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner: inner,
		Store: store,
		KeyFunc: func(req chatRequest) string {
			return req.SessionID
		},
		Inject: func(req chatRequest, state *chatState) chatRequest {
			// state is nil on first call — nothing to inject
			return req
		},
		Extract: func(req chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: resp.History}
		},
		TTL: 5 * time.Minute,
	})

	ctx := context.Background()
	resp, err := stateful.Execute(ctx, chatRequest{SessionID: "s1", Message: "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Reply != "echo:hello" {
		t.Fatalf("expected 'echo:hello', got %q", resp.Reply)
	}

	// Verify state was saved
	state, err := store.Load(ctx, "s1")
	if err != nil || state == nil {
		t.Fatalf("expected saved state, got %v, err %v", state, err)
	}
	if len(state.Messages) != 1 || state.Messages[0] != "hello" {
		t.Fatalf("expected [hello], got %v", state.Messages)
	}
}

func TestStateful_StateInjection(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	// Pre-seed state
	existing := chatState{Messages: []string{"previous"}}
	store.Save(context.Background(), "s1", &existing, 0)

	var injectedMessages []string

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner: inner,
		Store: store,
		KeyFunc: func(req chatRequest) string {
			return req.SessionID
		},
		Inject: func(req chatRequest, state *chatState) chatRequest {
			if state != nil {
				injectedMessages = state.Messages
			}
			return req
		},
		Extract: func(req chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: append(injectedMessages, req.Message)}
		},
		TTL: 0,
	})

	ctx := context.Background()
	_, err := stateful.Execute(ctx, chatRequest{SessionID: "s1", Message: "new"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(injectedMessages) != 1 || injectedMessages[0] != "previous" {
		t.Fatalf("expected injected [previous], got %v", injectedMessages)
	}

	// Check updated state
	state, _ := store.Load(ctx, "s1")
	if state == nil || len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", state)
	}
}

func TestStateful_NilStateFirstCall(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	var receivedNilState bool
	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner: inner,
		Store: store,
		KeyFunc: func(req chatRequest) string {
			return req.SessionID
		},
		Inject: func(req chatRequest, state *chatState) chatRequest {
			if state == nil {
				receivedNilState = true
			}
			return req
		},
		Extract: func(req chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: []string{req.Message}}
		},
	})

	_, err := stateful.Execute(context.Background(), chatRequest{SessionID: "new-session", Message: "first"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !receivedNilState {
		t.Fatal("expected nil state on first call")
	}
}

func TestStateful_NilExtractSkipsSave(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner: inner,
		Store: store,
		KeyFunc: func(req chatRequest) string {
			return req.SessionID
		},
		Inject: func(req chatRequest, _ *chatState) chatRequest {
			return req
		},
		Extract: func(_ chatRequest, _ chatResponse) *chatState {
			return nil // Don't persist state
		},
	})

	_, err := stateful.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// No state should be saved
	if store.Len() != 0 {
		t.Fatalf("expected no stored state, got %d entries", store.Len())
	}
}

func TestStateful_DelegatesNameAndAvailability(t *testing.T) {
	inner := &chatProvider{}
	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   inner,
		Store:   provider.NewMemoryStore[chatState](),
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, _ chatResponse) *chatState { return nil },
	})

	if stateful.Name() != "chat" {
		t.Fatalf("expected 'chat', got %q", stateful.Name())
	}
	if !stateful.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}
}

// --- Error propagation tests ---

type failingChatProvider struct{}

func (p *failingChatProvider) Name() string                       { return "fail-chat" }
func (p *failingChatProvider) IsAvailable(_ context.Context) bool { return true }
func (p *failingChatProvider) Execute(_ context.Context, _ chatRequest) (chatResponse, error) {
	return chatResponse{}, errors.New("provider error")
}

func TestStateful_InnerError(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   &failingChatProvider{},
		Store:   store,
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, _ chatResponse) *chatState { return nil },
	})

	_, err := stateful.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hello"})
	if err == nil {
		t.Fatal("expected error from inner provider")
	}

	// State should NOT be saved on error
	if store.Len() != 0 {
		t.Fatalf("expected no stored state after error, got %d entries", store.Len())
	}
}

func TestStateful_ComposesWithResilience(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()
	inner := &chatProvider{}

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   inner,
		Store:   store,
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: resp.History}
		},
	})

	// Wrap with resilience — Stateful implements RequestResponse
	resilient := provider.WithResilience[chatRequest, chatResponse](stateful, provider.ResilienceConfig{})

	resp, err := resilient.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hi"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp.Reply != "echo:hi" {
		t.Fatalf("expected 'echo:hi', got %q", resp.Reply)
	}
}

func TestStateful_MultipleCallsSameKey(t *testing.T) {
	store := provider.NewMemoryStore[chatState]()

	var callHistory []string

	// Custom inner that uses injected history
	inner := &historyAwareProvider{history: &callHistory}

	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   inner,
		Store:   store,
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject: func(req chatRequest, state *chatState) chatRequest {
			if state != nil {
				// Make history available to the inner provider via a side channel
				callHistory = state.Messages
			} else {
				callHistory = nil
			}
			return req
		},
		Extract: func(req chatRequest, _ chatResponse) *chatState {
			return &chatState{Messages: append(callHistory, req.Message)}
		},
		TTL: time.Minute,
	})

	ctx := context.Background()

	// First call
	stateful.Execute(ctx, chatRequest{SessionID: "s1", Message: "first"})

	// Second call — should have "first" in history
	stateful.Execute(ctx, chatRequest{SessionID: "s1", Message: "second"})

	state, _ := store.Load(ctx, "s1")
	if state == nil || len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", state)
	}
	if state.Messages[0] != "first" || state.Messages[1] != "second" {
		t.Fatalf("expected [first second], got %v", state.Messages)
	}
}

// historyAwareProvider uses side-channel to verify injection
type historyAwareProvider struct {
	history *[]string
}

func (p *historyAwareProvider) Name() string                       { return "history-aware" }
func (p *historyAwareProvider) IsAvailable(_ context.Context) bool { return true }
func (p *historyAwareProvider) Execute(_ context.Context, req chatRequest) (chatResponse, error) {
	return chatResponse{
		Reply:   fmt.Sprintf("processed:%s (history:%d)", req.Message, len(*p.history)),
		History: append(*p.history, req.Message),
	}, nil
}

// --- Store error tests ---

// errorStore always returns errors on Load/Save/Delete.
type errorStore struct {
	loadErr   error
	saveErr   error
	deleteErr error
}

func (s *errorStore) Load(_ context.Context, _ string) (*chatState, error) {
	return nil, s.loadErr
}
func (s *errorStore) Save(_ context.Context, _ string, _ *chatState, _ time.Duration) error {
	return s.saveErr
}
func (s *errorStore) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

func TestStateful_StoreLoadError(t *testing.T) {
	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   &chatProvider{},
		Store:   &errorStore{loadErr: errors.New("load failed")},
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, _ chatResponse) *chatState { return nil },
	})

	_, err := stateful.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hello"})
	if err == nil {
		t.Fatal("expected load error")
	}
	if err.Error() != "load failed" {
		t.Fatalf("expected 'load failed', got %q", err.Error())
	}
}

func TestStateful_StoreSaveError(t *testing.T) {
	stateful := provider.NewStateful(provider.StatefulConfig[chatRequest, chatResponse, chatState]{
		Inner:   &chatProvider{},
		Store:   &errorStore{saveErr: errors.New("save failed")},
		KeyFunc: func(req chatRequest) string { return req.SessionID },
		Inject:  func(req chatRequest, _ *chatState) chatRequest { return req },
		Extract: func(_ chatRequest, resp chatResponse) *chatState {
			return &chatState{Messages: resp.History}
		},
	})

	_, err := stateful.Execute(context.Background(), chatRequest{SessionID: "s1", Message: "hello"})
	if err == nil {
		t.Fatal("expected save error")
	}
	if err.Error() != "save failed" {
		t.Fatalf("expected 'save failed', got %q", err.Error())
	}
}
