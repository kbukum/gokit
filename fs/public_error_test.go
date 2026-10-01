package fs

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

func TestFilesystemDiagnosticIsNotPublic(t *testing.T) {
	t.Parallel()
	path := "/private/tenant/config.json"
	cause := &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
	for name, err := range map[string]error{
		"watch":          watchError("watch", path, cause),
		"permissions":    accessError("read permissions", path, cause),
		"canonicalize":   canonicalizeError(path, cause),
		"archive write":  archiveIOError(path, "write archive", cause),
		"archive read":   extractIOError(path, "read member", cause),
		"archive format": archiveFormatError(path, "decode member", cause),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertPrivateDiagnostic(t, err, cause, path)
		})
	}
	for name, err := range map[string]error{
		"unsafe member": escapeError(path, "/private/member"),
		"link member":   unsafeLinkError(path, "/private/member"),
		"size limit":    oversizeError(path, 42),
		"entry limit":   tooManyEntriesError(path, 42),
		"outside root":  ensureConfined("/private/root", path),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertPrivateDiagnostic(t, err, nil, "/private/")
		})
	}
}

func assertPrivateDiagnostic(t *testing.T, err, cause error, private string) {
	t.Helper()
	appErr := apperrors.Normalize(err)
	if appErr.Cause == nil {
		t.Fatal("diagnostic cause was lost")
	}
	if cause != nil && !errors.Is(appErr, cause) {
		t.Fatal("diagnostic cause was lost")
	}
	body, marshalErr := json.Marshal(appErr.ToProblemDetail())
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	wire, encodeErr := errorrpc.Encode(appErr, "")
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if strings.Contains(string(body), private) || strings.Contains(wire.String(), private) {
		t.Fatalf("internal path was serialized: %s", body)
	}
}
