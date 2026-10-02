package sse

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Publisher is the typed publication seam implemented by Bus and future shared-bus adapters.
type Publisher interface {
	Publish(ctx context.Context, pattern string, message proto.Message) error
}
