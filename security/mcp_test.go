package security_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/security"
)

func TestMCPHTTPHelpers(t *testing.T) {
	if err := security.ValidateLocalBind("127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if err := security.ValidateLocalBind("0.0.0.0:8080"); err == nil {
		t.Fatal("expected non-local bind error")
	}
	if err := security.ValidateLocalBind(":8080"); err == nil {
		t.Fatal("expected empty host bind error")
	}
	req, _ := http.NewRequest(http.MethodPost, "http://localhost", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://app.example")
	if err := security.ValidateOrigin(req, []string{"https://app.example"}); err != nil {
		t.Fatal(err)
	}
	if err := security.ValidateOrigin(req, []string{"https://other.example"}); err == nil {
		t.Fatal("expected origin error")
	}
	req.ContentLength = 3
	if err := security.EnforcePayloadLimit(httptest.NewRecorder(), req, 10); err != nil {
		t.Fatal(err)
	}
	req.ContentLength = 11
	if err := security.EnforcePayloadLimit(httptest.NewRecorder(), req, 10); err == nil {
		t.Fatal("expected payload limit error")
	}
	if err := (security.WarnOnlyVerifier{}).Verify(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
}
