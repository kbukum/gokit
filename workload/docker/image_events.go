package docker

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"

	"github.com/kbukum/gokit/workload"
)

// WatchImageEvents streams image lifecycle events (pull, delete, tag, untag).
func (m *Manager) WatchImageEvents(ctx context.Context, filter workload.ImageEventFilter) (<-chan workload.ImageEvent, error) {
	f := filters.NewArgs()
	f.Add("type", string(events.ImageEventType))
	for _, action := range filter.Actions {
		f.Add("event", action)
	}

	eventCh, errCh := m.client.Events(ctx, events.ListOptions{Filters: f})

	out := make(chan workload.ImageEvent, 64)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errCh:
				if !ok {
					return
				}
				m.log.Error("docker image event stream error", map[string]interface{}{"error": err.Error()})
				return
			case evt, ok := <-eventCh:
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
	if evt.From != "" {
		return evt.From
	}
	return evt.Actor.ID
}

// eventTimestamp converts a Docker event to a Go time, preferring nanosecond
// precision when available.
func eventTimestamp(evt events.Message) time.Time {
	if evt.TimeNano > 0 {
		return time.Unix(0, evt.TimeNano)
	}
	return time.Unix(evt.Time, 0)
}
