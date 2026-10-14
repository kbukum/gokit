package gcs

import (
	"context"
	"errors"
	"strings"
	"testing"

	gcstorage "cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/kbukum/gokit/storage"
)

func TestGoogleClientCloseIsRepeatableAndRefusesLaterUse(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client, err := gcstorage.NewClient(ctx, option.WithEndpoint("http://127.0.0.1:0/storage/v1/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	gc := &googleClient{bucket: "objects", client: client, cfg: &Config{Bucket: "objects"}}
	for range 2 {
		if err := gc.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	_, getErr := gc.Get(ctx, "a")
	_, existsErr := gc.Exists(ctx, "a")
	_, listErr := gc.List(ctx, "")
	_, headErr := gc.Head(ctx, "a")
	for name, err := range map[string]error{
		"put": gc.Put(ctx, "a", strings.NewReader("x")), "get": getErr, "delete": gc.Delete(ctx, "a"),
		"exists": existsErr, "list": listErr, "head": headErr, "copy": gc.Copy(ctx, "a", "b"),
	} {
		if !errors.Is(err, errClientClosed) {
			t.Errorf("%s after close = %v", name, err)
		}
	}
}

type closingClient struct {
	*fakeObjectClient
	calls int
	fail  error
}

func (c *closingClient) Close() error {
	c.calls++
	return c.fail
}

func TestStorageCloseForwardsOwnedClientRelease(t *testing.T) {
	t.Parallel()
	cause := errors.New("release failed")
	client := &closingClient{fakeObjectClient: newFakeClient(), fail: cause}
	var closer storage.Closer = NewStorageWithClient("objects", "", client)
	if err := closer.Close(t.Context()); !errors.Is(err, cause) {
		t.Fatalf("failed release hid cause: %v", err)
	}
	client.fail = nil
	if err := closer.Close(context.WithoutCancel(t.Context())); err != nil || client.calls != 2 {
		t.Fatalf("retry after failed release: calls=%d err=%v", client.calls, err)
	}
}
