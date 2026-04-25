package messaging

import "testing"

func FuzzParseData(f *testing.F) {
	f.Add([]byte(`{"id":1}`))
	f.Add([]byte(`not-json`))

	type payload struct {
		ID int `json:"id"`
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseData[payload](Event{Data: data})
	})
}
