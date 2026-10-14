package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
)

func TestExistsPreservesFailure(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead {
					t.Error("existence check did not use HEAD")
				}
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			s := newTestStorage(t, server.URL)
			t.Cleanup(func() {
				if err := s.Close(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})

			exists, err := s.Exists(t.Context(), "object")
			var response *awshttp.ResponseError
			if exists || !errors.As(err, &response) || response.HTTPStatusCode() != status {
				t.Fatalf("storage failure became absence or lost its cause: exists=%v, error=%v", exists, err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if exists, err := s.Exists(ctx, "object"); exists || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation became absence: exists=%v, error=%v", exists, err)
			}
		})
	}
}
