package testhost

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kbukum/gokit/codec"
)

//go:embed testdata/host.json
var hostContract string

func TestPublishedFixtureMatchesLiveProtocol(t *testing.T) {
	t.Parallel()
	contract, err := codec.Decode[struct {
		Protocol  string `json:"protocol"`
		Readiness struct {
			Method                 string   `json:"method"`
			Path                   string   `json:"path"`
			Status                 int      `json:"status"`
			SchemaVersion          uint     `json:"schemaVersion"`
			RequiredIdentityFields []string `json:"requiredIdentityFields"`
		} `json:"readiness"`
		RPC struct {
			Procedure string `json:"procedure"`
			Input     string `json:"input"`
			Output    string `json:"output"`
			Result    string `json:"result"`
		} `json:"rpc"`
		Events struct {
			Method                 string `json:"method"`
			Path                   string `json:"path"`
			ContentType            string `json:"contentType"`
			LocalReleaseMS         int    `json:"localReleaseMs"`
			CrossInstanceReleaseMS int    `json:"crossInstanceReleaseMs"`
		} `json:"events"`
		Browser struct {
			Cookie struct {
				Name     string `json:"name"`
				Path     string `json:"path"`
				Secure   bool   `json:"secure"`
				HTTPOnly bool   `json:"httpOnly"`
				SameSite string `json:"sameSite"`
				Domain   string `json:"domain"`
			} `json:"cookie"`
			CSRFHeader                      string          `json:"csrfHeader"`
			Login                           string          `json:"login"`
			Status                          string          `json:"status"`
			Logout                          string          `json:"logout"`
			RefreshEndpoint                 json.RawMessage `json:"refreshEndpoint"`
			ResetBeforeSignIn               bool            `json:"resetBeforeSignIn"`
			SeparateBrowserContextPerWorker bool            `json:"separateBrowserContextPerWorker"`
		} `json:"browser"`
		Control struct {
			Header    string   `json:"header"`
			Reset     string   `json:"reset"`
			Scenario  string   `json:"scenario"`
			Scenarios []string `json:"scenarios"`
		} `json:"control"`
	}](codec.CompactJSON(), hostContract)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Protocol != ProtocolVersion || contract.RPC.Procedure != OperationPath || contract.Events.Path != EventsPath {
		t.Fatal("published protocol constants drifted")
	}
	if contract.RPC.Input != string((&emptypb.Empty{}).ProtoReflect().Descriptor().FullName()) ||
		contract.RPC.Output != string((&wrapperspb.StringValue{}).ProtoReflect().Descriptor().FullName()) {
		t.Fatal("published protobuf types drifted")
	}
	cookie := contract.Browser.Cookie
	if cookie.Name != "__Host-session" || cookie.Path != "/" || !cookie.Secure || !cookie.HTTPOnly || cookie.SameSite != "Strict" || cookie.Domain != "" {
		t.Fatal("published cookie protections drifted")
	}
	if contract.Browser.Login != "/auth/login" || contract.Browser.Status != "/auth/session" || contract.Browser.Logout != "/auth/logout" ||
		string(contract.Browser.RefreshEndpoint) != "null" || !contract.Browser.ResetBeforeSignIn || !contract.Browser.SeparateBrowserContextPerWorker {
		t.Fatal("published browser lifecycle drifted")
	}
	if contract.Control.Header != "X-Test-Control" || contract.Control.Reset != "/_test/reset" || contract.Control.Scenario != "/_test/scenario" ||
		!slices.Equal(contract.Control.Scenarios, []string{"unavailable-store", "healthy-store", "expired", "hold-status", "release-status"}) {
		t.Fatal("published runner controls drifted")
	}
	if contract.Events.LocalReleaseMS != 1000 || contract.Events.CrossInstanceReleaseMS != 4000 ||
		!slices.Equal(contract.Readiness.RequiredIdentityFields, []string{"protocol", "runId", "buildId", "schemaVersion"}) {
		t.Fatal("published ownership bounds or readiness identity drifted")
	}
	host, client, fixture := liveHost(t)
	ready := request(t, client, contract.Readiness.Method, host.Origin()+contract.Readiness.Path, "", "")
	data, err := io.ReadAll(io.LimitReader(ready.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := codec.Decode[Ready](codec.CompactJSON(), string(data))
	if err != nil {
		t.Fatal(err)
	}
	if ready.StatusCode != contract.Readiness.Status || identity.Protocol != contract.Protocol ||
		identity.SchemaVersion != contract.Readiness.SchemaVersion || identity.RunID != host.config.RunID || identity.BuildID != host.config.BuildID {
		t.Fatal("actual readiness does not match the published fixture")
	}
	document := login(t, host, client, fixture)
	rpc := connect.NewClient[emptypb.Empty, wrapperspb.StringValue](client, host.Origin()+contract.RPC.Procedure)
	call := connect.NewRequest(&emptypb.Empty{})
	call.Header().Set(contract.Browser.CSRFHeader, document.CSRFToken)
	response, err := rpc.CallUnary(t.Context(), call)
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetValue() != contract.RPC.Result {
		t.Fatal("actual RPC does not match the published fixture")
	}
	stream := request(t, client, contract.Events.Method, host.Origin()+contract.Events.Path, "", "")
	if stream.StatusCode != http.StatusOK || stream.Header.Get("Content-Type") != contract.Events.ContentType {
		t.Fatal("actual stream does not match the published fixture")
	}
	closed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(closed) }()
	logout := request(t, client, http.MethodPost, host.Origin()+contract.Browser.Logout, "{}", document.CSRFToken)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatal("published logout endpoint failed")
	}
	select {
	case <-closed:
	case <-time.After(time.Duration(contract.Events.LocalReleaseMS) * time.Millisecond):
		t.Fatal("actual stream exceeded its published release bound")
	}
	status := request(t, client, http.MethodGet, host.Origin()+contract.Browser.Status, "", "")
	if status.StatusCode != http.StatusUnauthorized || len(status.Cookies()) != 0 {
		t.Fatal("published status endpoint renewed a logged-out session")
	}
}
