package process

import "bytes"

type limitedBuffer struct {
	max       int
	buf       bytes.Buffer
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{max: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		_, err := b.buf.Write(p)
		return len(p), err
	}

	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated = true
			return len(p), nil
		}
		_, _ = b.buf.Write(p)
		return len(p), nil
	}

	b.truncated = true
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}
