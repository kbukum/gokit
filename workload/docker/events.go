package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"

	"github.com/kbukum/gokit/workload"
)

// WatchEvents watches Docker container lifecycle events and emits WorkloadEvents.
func (m *Manager) WatchEvents(ctx context.Context, filter workload.ListFilter) (<-chan workload.WorkloadEvent, error) {
	f := filters.NewArgs()
	f.Add("type", string(events.ContainerEventType))
	for k, v := range filter.Labels {
		f.Add("label", fmt.Sprintf("%s=%s", k, v))
	}
	if filter.Name != "" {
		f.Add("container", filter.Name)
	}

	eventCh, errCh := m.client.Events(ctx, events.ListOptions{Filters: f})

	out := make(chan workload.WorkloadEvent, 64)
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
				m.log.Error("docker event stream error", map[string]interface{}{"error": err.Error()})
				return
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				we := workload.WorkloadEvent{
					ID:        evt.Actor.ID,
					Name:      evt.Actor.Attributes["name"],
					Event:     mapDockerEvent(evt.Action),
					Timestamp: time.Unix(evt.Time, evt.TimeNano),
					Message:   string(evt.Action),
				}
				select {
				case out <- we:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// mapDockerEvent normalizes Docker event actions to workload event names.
func mapDockerEvent(action events.Action) string {
	switch action {
	case events.ActionStart:
		return "start"
	case events.ActionStop:
		return "stop"
	case events.ActionDie:
		return "die"
	case events.ActionKill:
		return "kill"
	case events.ActionRestart:
		return "restart"
	case events.ActionOOM:
		return "oom"
	case events.ActionCreate:
		return "create"
	case events.ActionDestroy:
		return "destroy"
	case events.ActionPause:
		return "pause"
	case events.ActionUnPause:
		return "unpause"
	default:
		// health_status and others come as string
		return string(action)
	}
}
