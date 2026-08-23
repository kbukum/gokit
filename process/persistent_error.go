package process

import goerrors "github.com/kbukum/gokit/errors"

// persistentStartErrorKindDetail is the RFC 9457 extension member key under which a
// persistent startup failure classification is carried on an AppError.
const persistentStartErrorKindDetail = "gokit.process.persistent_start_error_kind"

// PersistentStartErrorKind is a machine-readable classification of why a persistent
// process failed to start and become ready.
type PersistentStartErrorKind string

const (
	// PersistentStartSpawnFailed indicates the persistent process could not be spawned.
	PersistentStartSpawnFailed PersistentStartErrorKind = "spawn_failed"
	// PersistentStartReadinessTimedOut indicates the process did not become ready before
	// the readiness timeout elapsed.
	PersistentStartReadinessTimedOut PersistentStartErrorKind = "readiness_timed_out"
	// PersistentStartExitedBeforeReadiness indicates the process exited before it became ready.
	PersistentStartExitedBeforeReadiness PersistentStartErrorKind = "exited_before_readiness"
	// PersistentStartOutputEndedBeforeReadiness indicates the output streams ended before
	// output readiness was observed.
	PersistentStartOutputEndedBeforeReadiness PersistentStartErrorKind = "output_ended_before_readiness"
)

// StartErrorKind returns the persistent startup failure classification attached to an
// error, reporting false when the error carries no such classification.
func StartErrorKind(err error) (PersistentStartErrorKind, bool) {
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		return "", false
	}
	raw, ok := appErr.Details[persistentStartErrorKindDetail].(string)
	if !ok {
		return "", false
	}
	switch kind := PersistentStartErrorKind(raw); kind {
	case PersistentStartSpawnFailed,
		PersistentStartReadinessTimedOut,
		PersistentStartExitedBeforeReadiness,
		PersistentStartOutputEndedBeforeReadiness:
		return kind, true
	default:
		return "", false
	}
}

// withStartErrorKind attaches a persistent startup failure classification to an AppError.
func withStartErrorKind(err *goerrors.AppError, kind PersistentStartErrorKind) *goerrors.AppError {
	return err.WithDetail(persistentStartErrorKindDetail, string(kind))
}
