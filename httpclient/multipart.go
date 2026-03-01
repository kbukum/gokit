package httpclient

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/textproto"
)

// MultipartBody represents a multipart/form-data request body.
// Pass this as the Body field of a Request to automatically construct
// multipart encoding with the correct Content-Type header.
type MultipartBody struct {
	// Fields are simple key-value form fields.
	Fields map[string]string
	// Files are file upload fields.
	Files []FileField
}

// FileField represents a file to upload in a multipart request.
type FileField struct {
	// FieldName is the form field name (e.g., "file", "audio").
	FieldName string
	// FileName is the file name sent to the server.
	FileName string
	// ContentType is the MIME type (e.g., "audio/wav"). If empty, uses application/octet-stream.
	ContentType string
	// Data is the file content. Used if Reader is nil.
	Data []byte
	// Reader is an alternative to Data for large files (streaming upload).
	Reader io.Reader
}

// encode builds the multipart body and returns the reader and content-type header.
func (m *MultipartBody) encode() (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Write form fields
	for k, v := range m.Fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}

	// Write file fields
	for _, f := range m.Files {
		var part io.Writer
		var err error

		if f.ContentType != "" {
			// Use custom content-type via CreatePart
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition",
				`form-data; name="`+escapeQuotes(f.FieldName)+`"; filename="`+escapeQuotes(f.FileName)+`"`)
			header.Set("Content-Type", f.ContentType)
			part, err = w.CreatePart(header)
		} else {
			part, err = w.CreateFormFile(f.FieldName, f.FileName)
		}
		if err != nil {
			return nil, "", err
		}

		if f.Data != nil {
			if _, err := part.Write(f.Data); err != nil {
				return nil, "", err
			}
		} else if f.Reader != nil {
			if _, err := io.Copy(part, f.Reader); err != nil {
				return nil, "", err
			}
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}

	return &buf, w.FormDataContentType(), nil
}

// escapeQuotes replaces special characters in header values.
func escapeQuotes(s string) string {
	var buf bytes.Buffer
	for _, b := range []byte(s) {
		if b == '"' || b == '\\' {
			buf.WriteByte('\\')
		}
		buf.WriteByte(b)
	}
	return buf.String()
}
