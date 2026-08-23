package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// collectStream drains an agent event stream into a slice, failing on timeout.
func collectStream(t *testing.T, ch <-chan agent.AgentEvent) []agent.AgentEvent {
	t.Helper()
	var events []agent.AgentEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt)
		case <-timeout:
			t.Fatal("stream did not close within 2s")
		}
	}
}

func TestStreamEmitsTurnLifecycleAndTokenDeltas(t *testing.T) {
	p := newMockProvider(textResponse("Hello!"))
	a := agent.New(agent.Config{Provider: p})

	ch, err := a.Stream(context.Background(), []chat.Message{chat.User("hi")})
	if err != nil {
		t.Fatal(err)
	}
	events := collectStream(t, ch)

	var (
		sawTurnStart, sawTurnComplete, sawDelta, sawComplete bool
		completed                                            *agent.RunComplete
	)
	for _, evt := range events {
		switch e := evt.(type) {
		case agent.TurnStart:
			sawTurnStart = true
		case agent.TurnComplete:
			sawTurnComplete = true
		case agent.LLMDelta:
			sawDelta = true
			if _, ok := e.Event.(llm.MessageComplete); ok {
				sawComplete = true
			}
		case agent.RunComplete:
			c := e
			completed = &c
		}
	}
	if !sawTurnStart || !sawTurnComplete || !sawDelta || !sawComplete {
		t.Fatalf("missing lifecycle/delta events: start=%v complete=%v delta=%v msgComplete=%v", sawTurnStart, sawTurnComplete, sawDelta, sawComplete)
	}
	if completed == nil || completed.Err != nil {
		t.Fatalf("expected terminal RunComplete without error, got %+v", completed)
	}
	if completed.Result == nil || completed.Result.StopReason != agent.StopEndTurn || completed.Result.FinalMessage.Text() != "Hello!" {
		t.Fatalf("unexpected final result: %+v", completed.Result)
	}
}

func TestStreamRunsFullToolLoop(t *testing.T) {
	p := newMockProvider(toolCallResponse("calculator", "{}"), textResponse("42"))
	a := agent.New(agent.Config{Provider: p, Tools: makeMockTool("calculator", "42")})

	ch, err := a.Stream(context.Background(), []chat.Message{chat.User("calc")})
	if err != nil {
		t.Fatal(err)
	}
	events := collectStream(t, ch)

	var (
		toolExecuting, toolComplete bool
		turnStarts                  int
		completed                   *agent.RunComplete
	)
	for _, evt := range events {
		switch e := evt.(type) {
		case agent.TurnStart:
			turnStarts++
		case agent.ToolExecuting:
			if e.Name == "calculator" {
				toolExecuting = true
			}
		case agent.ToolComplete:
			if e.Name == "calculator" && e.Err == nil {
				toolComplete = true
			}
		case agent.RunComplete:
			c := e
			completed = &c
		}
	}
	if turnStarts != 2 {
		t.Fatalf("want 2 turns, got %d", turnStarts)
	}
	if !toolExecuting || !toolComplete {
		t.Fatalf("missing tool events: executing=%v complete=%v", toolExecuting, toolComplete)
	}
	if completed == nil || completed.Result == nil || completed.Result.TurnCount != 2 || completed.Result.StopReason != agent.StopEndTurn {
		t.Fatalf("unexpected terminal result: %+v", completed)
	}
}

func TestStreamPropagatesCancellation(t *testing.T) {
	p := newMockProvider()
	p.blockUntilCancel = true
	a := agent.New(agent.Config{Provider: p, WallClock: 10 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Stream(ctx, []chat.Message{chat.User("wait")})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close the stream within 1s")
	}
}

// A consumer that stops reading must not leak the driving goroutine: the WallClock-bounded emit
// aborts the loop and closes the channel even though nothing drains it.
func TestStreamStalledConsumerAbortsOnWallClock(t *testing.T) {
	p := newMockProvider(textResponse("hello"))
	a := agent.New(agent.Config{Provider: p, WallClock: 150 * time.Millisecond, StreamBuffer: 1})

	ch, err := a.Stream(context.Background(), []chat.Message{chat.User("hi")})
	if err != nil {
		t.Fatal(err)
	}
	// Do not read until well past the wall clock, forcing emit to block once the buffer fills.
	time.Sleep(400 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stalled consumer leaked the stream goroutine")
	}
}
