package httpclient

import "testing"

func TestResponse_JSON_Empty(t *testing.T) {
	resp := &Response{Body: []byte{}}
	var v map[string]string
	err := resp.JSON(&v)
	if err == nil {
		t.Error("expected error decoding empty JSON body")
	}
}

func TestResponse_IsSuccess_Boundaries(t *testing.T) {
	tests := []struct {
		code    int
		success bool
		isErr   bool
	}{
		{199, false, false},
		{200, true, false},
		{299, true, false},
		{300, false, false},
		{399, false, false},
		{400, false, true},
		{500, false, true},
	}
	for _, tt := range tests {
		r := &Response{StatusCode: tt.code}
		if r.IsSuccess() != tt.success {
			t.Errorf("StatusCode %d: IsSuccess() = %v, want %v", tt.code, r.IsSuccess(), tt.success)
		}
		if r.IsError() != tt.isErr {
			t.Errorf("StatusCode %d: IsError() = %v, want %v", tt.code, r.IsError(), tt.isErr)
		}
	}
}

func TestStreamResponse_Close_NilFields(t *testing.T) {
	sr := &StreamResponse{}
	if err := sr.Close(); err != nil {
		t.Errorf("Close on nil stream = %v", err)
	}
}
