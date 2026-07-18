package docker

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"

	"github.com/kbukum/gokit/workload"
)

// WatchImageEvents streams image lifecycle events (pull, delete, tag, untag).
func (m *Manager) WatchImageEvents(ctx context.Context, filter workload.ImageEventFilter) (<-chan workload.ImageEvent, error) {
	f := make(client.Filters)
	f.Add("type", string(events.ImageEventType))
	for _, action := range filter.Actions {
		f.Add("event", action)
	}

	eventsResult := m.client.Events(ctx, client.EventsListOptions{Filters: f})

	out := make(chan workload.ImageEvent, 64)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-eventsResult.Err:
				if !ok {
					return
				}
				m.log.ErrorCtx(ctx, "docker image event stream error", map[string]any{"error": err.Error()})
				return
			case evt, ok := <-eventsResult.Messages:
				if !ok {
					return
				}
				ie := workload.ImageEvent{
					Action:    string(evt.Action),
					ImageID:   evt.Actor.ID,
					ImageRef:  imageRefFromEvent(evt),
					Timestamp: eventTimestamp(evt),
				}
				select {
				case out <- ie:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// imageRefFromEvent extracts the best image reference from a Docker event,
// falling back through available fields.
func imageRefFromEvent(evt events.Message) string {
	if name := evt.Actor.Attributes["name"]; name != "" {
		return name
	}
	if img := evt.Actor.Attributes["image"]; img != "" {
		return img
	}
	return evt.Actor.ID
}

// eventTimestamp converts a Docker event to a Go time,
// preferring nanosecond precision when available.
func eventTimestamp(evt events.Message) time.Time {
	if evt.TimeNano > 0 {
		return time.Unix(0, evt.TimeNano)
	}
	return time.Unix(evt.Time, 0)
}
