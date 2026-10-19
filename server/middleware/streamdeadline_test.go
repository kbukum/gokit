package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kbukum/gokit/util"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines   []time.Time
	events      []string
	deadlineErr error
	flushErr    error
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	if d.deadlineErr != nil {
		return d.deadlineErr
	}
	d.deadlines = append(d.deadlines, t)
	if t.IsZero() {
		d.events = append(d.events, "disarm")
	} else {
		d.events = append(d.events, "arm")
	}
	return nil
}

func (d *deadlineRecorder) Write(p []byte) (int, error) {
	d.events = append(d.events, "write")
	return d.ResponseRecorder.Write(p)
}

func (d *deadlineRecorder) FlushError() error {
	d.events = append(d.events, "flush")
	return d.flushErr
}

func TestStreamWriteDeadlineBoundsEachWriteAndDisarmsWhileIdle(t *testing.T) {
	clock := util.NewFakeClock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler := streamWriteDeadline(t, 2*time.Second, clock)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte("one"))
		assert.NoError(t, err)
		assert.NoError(t, http.NewResponseController(w).Flush())
		clock.Advance(time.Minute)
		_, err = w.Write([]byte("two"))
		assert.NoError(t, err)
	}))

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/stream", http.NoBody))

	assert.Equal(t, "onetwo", rec.Body.String())
	assert.Equal(t, []string{"disarm", "arm", "write", "disarm", "arm", "flush", "disarm", "arm", "write", "disarm"}, rec.events)
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.Equal(t, start.Add(2*time.Second), rec.deadlines[1])
	assert.Equal(t, start.Add(2*time.Second), rec.deadlines[3])
	assert.Equal(t, start.Add(time.Minute+2*time.Second), rec.deadlines[5])
}

func TestStreamWriteDeadlineRefusesWritersWithoutDeadlines(t *testing.T) {
	called := false
	handler := streamWriteDeadline(t, time.Second, util.SystemClock{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/stream", http.NoBody))

	assert.False(t, called)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStreamWriteDeadlineReportsDeadlineAndFlushFailures(t *testing.T) {
	failure := errors.New("deadline unavailable")
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	var w http.ResponseWriter
	streamWriteDeadline(t, time.Second, util.SystemClock{})(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		w = rw
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/stream", http.NoBody))

	rec.deadlineErr = failure
	n, err := w.Write([]byte("x"))
	assert.Zero(t, n)
	require.ErrorIs(t, err, failure)
	require.ErrorIs(t, http.NewResponseController(w).Flush(), failure)

	rec.deadlineErr, rec.flushErr = nil, io.ErrClosedPipe
	require.ErrorIs(t, http.NewResponseController(w).Flush(), io.ErrClosedPipe)
	assert.Equal(t, "disarm", rec.events[len(rec.events)-1], "a failed flush still disarms the deadline")
}

func TestStreamWriteDeadlineServesFlusherAndControllerCallers(t *testing.T) {
	clock := util.NewFakeClock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	own := clock.Now().Add(time.Hour)
	streamWriteDeadline(t, time.Second, clock)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if flusher, ok := w.(http.Flusher); assert.True(t, ok) {
			flusher.Flush()
		}
		assert.NoError(t, http.NewResponseController(w).SetWriteDeadline(own))
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/stream", http.NoBody))

	assert.Equal(t, []string{"disarm", "arm", "flush", "disarm", "arm"}, rec.events)
	assert.Equal(t, own, rec.deadlines[len(rec.deadlines)-1], "a handler keeps direct deadline control through Unwrap")
}

func TestStreamWriteDeadlineRejectsInvalidConfiguration(t *testing.T) {
	_, err := StreamWriteDeadline(0)
	require.Error(t, err)
	_, err = StreamWriteDeadline(time.Second, WithStreamDeadlineClock(nil))
	require.Error(t, err)
	_, err = StreamWriteDeadline(time.Second, nil)
	require.Error(t, err)
	_, err = StreamWriteDeadline(time.Second)
	require.NoError(t, err, "the system clock is the default")
}

func streamWriteDeadline(t *testing.T, budget time.Duration, clock util.Clock) Middleware {
	t.Helper()
	m, err := StreamWriteDeadline(budget, WithStreamDeadlineClock(clock))
	require.NoError(t, err)
	return m
}
