package memory

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/messaging"
)

// NewEvent is a convenience helper that creates an Event with auto-generated ID.
func NewEvent[D any](eventType, source string, data D, subject ...string) (messaging.Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return messaging.Event{}, fmt.Errorf("memory: marshal event data: %w", err)
	}
	e := messaging.Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      raw,
	}
	if len(subject) > 0 {
		e.Subject = subject[0]
	}
	return e, nil
}
