package testhost

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCookieAndAPIKeyCallSameRealConnectOperation(t *testing.T) {
	t.Parallel()
	host, browser, fixture := liveHost(t)
	document := login(t, host, browser, fixture)
	cookieClient := connect.NewClient[emptypb.Empty, wrapperspb.StringValue](browser, host.Origin()+OperationPath)
	input := connect.NewRequest(&emptypb.Empty{})
	input.Header().Set("X-CSRF-Token", document.CSRFToken)
	cookieResult, err := cookieClient.CallUnary(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	automation := *browser
	automation.Jar = nil
	keyClient := connect.NewClient[emptypb.Empty, wrapperspb.StringValue](&automation, host.Origin()+OperationPath)
	keyRequest := connect.NewRequest(&emptypb.Empty{})
	keyRequest.Header().Set("X-API-Key", host.APIKey())
	keyResult, err := keyClient.CallUnary(t.Context(), keyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if cookieResult.Msg.Value != "user:fixture-user" || keyResult.Msg.Value != cookieResult.Msg.Value {
		t.Fatal("automation and browser received different caller contracts")
	}
	ambiguous := connect.NewRequest(&emptypb.Empty{})
	ambiguous.Header().Set("X-API-Key", host.APIKey())
	ambiguous.Header().Set("X-CSRF-Token", document.CSRFToken)
	if _, err := cookieClient.CallUnary(t.Context(), ambiguous); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("ambiguous credentials did not fail through the Connect error contract")
	}
	badCSRF := connect.NewRequest(&emptypb.Empty{})
	if _, err := cookieClient.CallUnary(t.Context(), badCSRF); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatal("cookie CSRF rejection lost its Connect classification")
	}
}
