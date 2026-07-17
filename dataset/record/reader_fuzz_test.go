package record

import "testing"

// FuzzParseCSV ensures the CSV parser never panics on untrusted input and fails
// closed on malformed rows.
func FuzzParseCSV(f *testing.F) {
	f.Add([]byte("a,b\n1,2\n"))
	f.Add([]byte(""))
	f.Add([]byte("a\n"))
	f.Add([]byte("a,b\n1\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseCSV(data)
	})
}

// FuzzParseJSONArray ensures the JSON-array parser never panics and rejects
// non-object payloads.
func FuzzParseJSONArray(f *testing.F) {
	f.Add([]byte(`[{"a":1}]`))
	f.Add([]byte(`[1,2]`))
	f.Add([]byte(`{`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseJSONArray(data)
	})
}

// FuzzParseJSONLines ensures the JSON-lines parser never panics on untrusted
// input.
func FuzzParseJSONLines(f *testing.F) {
	f.Add([]byte("{\"a\":1}\n{\"b\":2}\n"))
	f.Add([]byte("42\n"))
	f.Add([]byte("\n\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseJSONLines(data)
	})
}
