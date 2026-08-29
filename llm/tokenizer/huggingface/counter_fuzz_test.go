package huggingface

import (
	"bytes"
	"testing"
)

func FuzzFromReaderDoesNotPanic(f *testing.F) {
	for _, seed := range [][]byte{
		nil,
		[]byte(""),
		[]byte("{"),
		[]byte(`{"model":{"type":"BPE","dropout":0.1}}`),
		[]byte(`{"model":{"type":"WordLevel","vocab":[]}}`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = FromReader(bytes.NewReader(data), 1<<20)
	})
}
