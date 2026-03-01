package httpclient

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultipartBody_Encode_FieldsOnly(t *testing.T) {
	mp := &MultipartBody{
		Fields: map[string]string{
			"name":  "test",
			"value": "hello",
		},
	}

	reader, contentType, err := mp.encode()
	if err != nil {
		t.Fatalf("encode() error: %v", err)
	}
	if reader == nil {
		t.Fatal("encode() returned nil reader")
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType error: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Errorf("media type = %q, want multipart/form-data", mediaType)
	}

	mr := multipart.NewReader(reader, params["boundary"])
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart error: %v", err)
		}
		data, _ := io.ReadAll(part)
		fields[part.FormName()] = string(data)
	}

	if fields["name"] != "test" || fields["value"] != "hello" {
		t.Errorf("fields = %v, want name=test, value=hello", fields)
	}
}

func TestMultipartBody_Encode_WithFile(t *testing.T) {
	fileData := []byte("audio data here")
	mp := &MultipartBody{
		Fields: map[string]string{"language": "en"},
		Files: []FileField{
			{FieldName: "file", FileName: "audio.wav", Data: fileData},
		},
	}

	reader, contentType, err := mp.encode()
	if err != nil {
		t.Fatalf("encode() error: %v", err)
	}

	_, params, _ := mime.ParseMediaType(contentType)
	mr := multipart.NewReader(reader, params["boundary"])

	var gotField, gotFile bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart error: %v", err)
		}

		if part.FormName() == "language" {
			data, _ := io.ReadAll(part)
			if string(data) != "en" {
				t.Errorf("language field = %q, want %q", data, "en")
			}
			gotField = true
		}

		if part.FormName() == "file" {
			if part.FileName() != "audio.wav" {
				t.Errorf("filename = %q, want %q", part.FileName(), "audio.wav")
			}
			data, _ := io.ReadAll(part)
			if !bytes.Equal(data, fileData) {
				t.Errorf("file data = %q, want %q", data, fileData)
			}
			gotFile = true
		}
	}

	if !gotField {
		t.Error("language field not found")
	}
	if !gotFile {
		t.Error("file field not found")
	}
}

func TestMultipartBody_Encode_WithFileContentType(t *testing.T) {
	mp := &MultipartBody{
		Files: []FileField{
			{
				FieldName:   "audio",
				FileName:    "speech.wav",
				ContentType: "audio/wav",
				Data:        []byte("wav data"),
			},
		},
	}

	reader, _, err := mp.encode()
	if err != nil {
		t.Fatalf("encode() error: %v", err)
	}

	// Verify the content-type is set on the part
	data, _ := io.ReadAll(reader)
	if !bytes.Contains(data, []byte("Content-Type: audio/wav")) {
		t.Error("expected Content-Type: audio/wav in multipart body")
	}
}

func TestMultipartBody_Encode_WithReader(t *testing.T) {
	content := "streamed content"
	mp := &MultipartBody{
		Files: []FileField{
			{
				FieldName: "file",
				FileName:  "data.txt",
				Reader:    bytes.NewReader([]byte(content)),
			},
		},
	}

	reader, contentType, err := mp.encode()
	if err != nil {
		t.Fatalf("encode() error: %v", err)
	}

	_, params, _ := mime.ParseMediaType(contentType)
	mr := multipart.NewReader(reader, params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("NextPart error: %v", err)
	}

	data, _ := io.ReadAll(part)
	if string(data) != content {
		t.Errorf("file content = %q, want %q", data, content)
	}
}

func TestAdapter_Do_Multipart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		ct := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("ParseMediaType error: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Errorf("Content-Type = %q, want multipart/form-data", mediaType)
		}

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}

		if got := r.FormValue("model"); got != "large-v3" {
			t.Errorf("model field = %q, want %q", got, "large-v3")
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile error: %v", err)
		}
		defer file.Close()

		if header.Filename != "audio.wav" {
			t.Errorf("filename = %q, want %q", header.Filename, "audio.wav")
		}

		data, _ := io.ReadAll(file)
		if string(data) != "audio bytes" {
			t.Errorf("file data = %q, want %q", data, "audio bytes")
		}

		w.WriteHeader(200)
		w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer srv.Close()

	adapter, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	resp, err := adapter.Do(t.Context(), Request{
		Method: http.MethodPost,
		Path:   "/transcribe",
		Body: &MultipartBody{
			Fields: map[string]string{"model": "large-v3"},
			Files: []FileField{
				{FieldName: "file", FileName: "audio.wav", Data: []byte("audio bytes")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if got := resp.Text(); got != `{"text":"hello world"}` {
		t.Errorf("body = %q, want json", got)
	}
}
