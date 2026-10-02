package sse

import (
	"strconv"
	"strings"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
)

// Event is an immutable encoded application or control event. Only application events carry an ID.
type Event struct {
	ID   string
	Name string
	Data string
}

// Wire returns the complete SSE frame, including its terminating empty line.
func (e Event) Wire() string {
	var b strings.Builder
	b.Grow(e.size())
	if e.ID != "" {
		b.WriteString("id: ")
		b.WriteString(e.ID)
		b.WriteByte('\n')
	}
	b.WriteString("event: ")
	b.WriteString(e.Name)
	b.WriteString("\ndata: ")
	b.WriteString(e.Data)
	b.WriteString("\n\n")
	return b.String()
}

func (e Event) size() int {
	size := len("event: \ndata: \n\n") + len(e.Name) + len(e.Data)
	if e.ID != "" {
		size += len("id: \n") + len(e.ID)
	}
	return size
}

// Connected identifies the instance and the atomic subscription boundary. Its cursor is not a delivery acknowledgement.
type Connected struct {
	Epoch  string `json:"epoch"`
	Cursor string `json:"cursor"`
}

// Reset invalidates the client's snapshot generation. The cursor identifies the new live boundary, not an applied application event.
type Reset struct {
	Reason string `json:"reason"`
	Cursor string `json:"cursor"`
}

func controlEvent[T any](name string, payload T) (Event, error) {
	data, err := codec.Encode(codec.CompactJSON(), payload)
	if err != nil {
		return Event{}, err
	}
	return Event{Name: name, Data: data}, nil
}

func resetEvent(reason, cursor string) Event {
	// Both fields are internal ASCII protocol values, never caller-provided JSON.
	return Event{Name: "reset", Data: `{"reason":"` + reason + `","cursor":"` + cursor + `"}`}
}

func (b *Bus) resume(cursor string) (after uint64, reset string, failure error) {
	if cursor == "" {
		return b.sequence, "", nil
	}
	epoch, seq, ok := strings.Cut(cursor, ":")
	if !ok || len(epoch) != 32 || strings.Trim(epoch, "0123456789abcdef") != "" || seq == "" || len(seq) > 20 {
		return 0, "", apperrors.InvalidInput("cursor", "invalid SSE cursor")
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != seq {
		return 0, "", apperrors.InvalidInput("cursor", "invalid SSE cursor")
	}
	if epoch != b.epoch {
		return b.sequence, "epochChanged", nil
	}
	if n > b.sequence {
		return 0, "", apperrors.InvalidInput("cursor", "SSE cursor is ahead of this instance")
	}
	if b.count > 0 && n < b.replay[b.head].sequence-1 {
		return b.sequence, "replayExpired", nil
	}
	return n, "", nil
}
