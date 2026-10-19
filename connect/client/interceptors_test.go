package client

import (
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
)

func TestInterceptorsPutAvailabilityFirst(t *testing.T) {
	availability := NewAvailability("peer")
	chain, err := Interceptors(WithAvailability(availability), WithUnaryTimeout(time.Second), WithFirstMessageTimeout(time.Second, WithFirstMessageClock(newArmedClock())))
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 || chain[0] != connect.Interceptor(availability) {
		t.Fatalf("chain = %#v", chain)
	}
	if _, ok := chain[1].(unaryTimeout); !ok {
		t.Fatalf("second = %T", chain[1])
	}
	if _, ok := chain[2].(*firstMessage); !ok {
		t.Fatalf("third = %T", chain[2])
	}
	alone, err := Interceptors(WithAvailability(availability))
	if err != nil || len(alone) != 1 {
		t.Fatalf("alone = %v, %v", alone, err)
	}
	timeoutOnly, err := Interceptors(WithUnaryTimeout(time.Second))
	if err != nil || len(timeoutOnly) != 1 {
		t.Fatalf("timeout only = %v, %v", timeoutOnly, err)
	}
	if _, ok := timeoutOnly[0].(unaryTimeout); !ok {
		t.Fatalf("timeout only = %T", timeoutOnly[0])
	}
	// Not set means no interceptor at all.
	if none, err := Interceptors(); err != nil || len(none) != 0 {
		t.Fatalf("none = %v, %v", none, err)
	}
	peer, err := NewPeer("peer", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := peer.Interceptors(); err != nil || got[0] != connect.Interceptor(peer.Availability()) {
		t.Fatalf("peer chain = %v, %v", got, err)
	}
}

func TestInterceptorsRefuseInvalidBounds(t *testing.T) {
	availability := NewAvailability("peer")
	for name, opts := range map[string][]InterceptorOption{
		"nil option":    {nil},
		"unary":         {WithUnaryTimeout(-time.Second)},
		"zero unary":    {WithUnaryTimeout(0)},
		"zero first":    {WithFirstMessageTimeout(0)},
		"first message": {WithFirstMessageTimeout(-time.Second)},
		"first clock":   {WithFirstMessageTimeout(time.Second, WithFirstMessageClock(nil))},
	} {
		if _, err := Interceptors(append([]InterceptorOption{WithAvailability(availability)}, opts...)...); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := Interceptors(WithAvailability(nil)); err == nil {
		t.Error("nil availability accepted")
	}
}
