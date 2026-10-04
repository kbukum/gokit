package testhost

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"time"

	sessiondb "github.com/kbukum/gokit/auth/session/database"
	"github.com/kbukum/gokit/codec"
	dbtestutil "github.com/kbukum/gokit/database/testutil"
	apperrors "github.com/kbukum/gokit/errors"
)

// Ready is the exact fixture readiness document. A runner must match its own run identity and the expected protocol/schema, not just an HTTP status.
type Ready struct {
	Protocol      string `json:"protocol"`
	RunID         string `json:"runId"`
	BuildID       string `json:"buildId"`
	SchemaVersion uint   `json:"schemaVersion"`
}

func (h *Host) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.store.check(ctx); err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := h.migrations.Ready(ctx, sessiondb.SchemaVersion); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.writeJSON(w, r, Ready{Protocol: ProtocolVersion, RunID: h.config.RunID, BuildID: h.config.BuildID, SchemaVersion: 1})
}

func (h *Host) permittedControl(w http.ResponseWriter, r *http.Request) bool {
	tokens := r.Header.Values("X-Test-Control")
	if len(tokens) != 1 || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(h.fixture.ControlToken)) != 1 {
		http.NotFound(w, r)
		return false
	}
	return true
}

func (h *Host) reset(w http.ResponseWriter, r *http.Request) {
	if !h.permittedControl(w, r) {
		return
	}
	if !h.activity.TryLock() {
		h.writeError(w, r, apperrors.New(apperrors.ErrCodeConflict, "Quiesce protected requests and streams before reset"))
		return
	}
	defer h.activity.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := dbtestutil.TruncateAllTables(ctx, h.db.GormDB); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.store.unavailable.Store(false)
	h.clock.Set(h.startedAt)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) scenario(w http.ResponseWriter, r *http.Request) {
	if !h.permittedControl(w, r) {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256))
	if err != nil {
		h.writeError(w, r, apperrors.InvalidInput("scenario", "Scenario input is too large"))
		return
	}
	input, err := codec.Decode[struct {
		Name string `json:"name"`
	}](codec.CompactJSON(), string(data))
	if err != nil {
		h.writeError(w, r, apperrors.InvalidInput("scenario", "Invalid scenario"))
		return
	}
	switch input.Name {
	case "unavailable-store":
		h.store.unavailable.Store(true)
	case "healthy-store":
		h.store.unavailable.Store(false)
	case "expired":
		h.clock.Advance(time.Hour)
	case "hold-status":
		if !h.status.CompareAndSwap(nil, &heldStatus{release: make(chan struct{})}) {
			h.writeError(w, r, apperrors.New(apperrors.ErrCodeConflict, "A status response is already held"))
			return
		}
	case "release-status":
		if held := h.status.Load(); held != nil {
			held.close()
		}
	default:
		h.writeError(w, r, apperrors.InvalidInput("scenario", "Unknown scenario"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) writeJSON(w http.ResponseWriter, r *http.Request, value Ready) {
	data, err := codec.Encode(codec.CompactJSON(), value)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.WriteString(w, data); err != nil {
		h.log.ErrorCtx(r.Context(), "Fixture response write failed", map[string]any{"error": err.Error()})
	}
}

func (h *Host) writeError(w http.ResponseWriter, r *http.Request, err error) {
	failure := apperrors.Normalize(err)
	data, encodeErr := codec.Encode(codec.CompactJSON(), failure.ToProblemDetail())
	if encodeErr != nil {
		h.log.ErrorCtx(r.Context(), "Fixture failure encoding failed", map[string]any{"error": encodeErr.Error()})
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(failure.HTTPStatus())
	if _, err := io.WriteString(w, data); err != nil {
		h.log.ErrorCtx(r.Context(), "Fixture failure write failed", map[string]any{"error": err.Error()})
	}
}
