package config

import "context"

// ChangeKind classifies a ConfigChange.
type ChangeKind int

const (
	// ChangeSet indicates the value at a key was created or replaced.
	ChangeSet ChangeKind = iota
	// ChangeRemoved indicates the value at a key was removed.
	ChangeRemoved
	// ChangeReloaded indicates the backend changed wholesale; consumers should
	// reload everything. Its Key is empty.
	ChangeReloaded
)

// ConfigChange is a typed configuration change emitted by a ConfigWatch source.
//
// It carries only the affected key, never the value, so change notifications are
// safe to log. Consumers react by re-running the load pipeline and re-decoding.
type ConfigChange struct {
	// Kind classifies the change.
	Kind ChangeKind
	// Key is the affected configuration key; empty for ChangeReloaded.
	Key string
}

// ConfigWatch is the contract for configuration backends that emit change
// notifications — a watched file, a remote config service, or an in-memory
// store. The returned channel is the bounded, owned change stream.
//
// The channel is buffered and best-effort; a consumer that cannot keep up may
// miss events and should treat any received change as a signal to reload. The
// channel is closed when ctx is canceled or the source is dropped, so ranging
// over it terminates cleanly.
type ConfigWatch interface {
	// Watch subscribes to configuration changes until ctx is canceled.
	Watch(ctx context.Context) (<-chan ConfigChange, error)
}

// watchBuffer bounds each subscriber's change channel. Sends are non-blocking:
// a lagging consumer drops events rather than stalling the writer.
const watchBuffer = 16

// Watch subscribes to changes on the in-memory sink. The returned channel
// receives a ConfigChange for every subsequent Set and Remove and is closed when
// ctx is canceled.
func (s *InMemoryConfigSink) Watch(ctx context.Context) (<-chan ConfigChange, error) {
	ch := make(chan ConfigChange, watchBuffer)

	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan ConfigChange]struct{})
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}()

	return ch, nil
}

// emit delivers a change to every subscriber. The caller must hold s.mu, which
// serializes emit against the Watch cleanup goroutine so a send never races a
// close. Delivery is non-blocking; a full subscriber buffer drops the event.
func (s *InMemoryConfigSink) emit(change ConfigChange) {
	for ch := range s.subs {
		select {
		case ch <- change:
		default:
		}
	}
}
