package media

import (
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

// maxDetectBytes is the number of bytes needed for reliable detection.
const maxDetectBytes = 4096

// Detect identifies the media type from raw bytes.
// It inspects at most the first [maxDetectBytes] bytes.
func Detect(data []byte) Info {
	if len(data) == 0 {
		return Info{Type: Unknown}
	}

	// Try binary format detection first (order: video, audio, image).
	if info, ok := detectVideo(data); ok {
		return info
	}
	if info, ok := detectAudio(data); ok {
		return info
	}
	if info, ok := detectImage(data); ok {
		return info
	}

	// Fall back to text heuristic.
	if isText(data) {
		return Info{Type: Text, Format: FormatText, MimeType: "text/plain"}
	}

	return Info{Type: Unknown}
}

// DetectReader reads up to [maxDetectBytes] from r and detects the media type.
func DetectReader(r io.Reader) (Info, error) {
	buf := make([]byte, maxDetectBytes)
	n, err := io.ReadAtLeast(r, buf, 1)
	if err != nil {
		return Info{}, fmt.Errorf("media: read failed: %w", err)
	}
	return Detect(buf[:n]), nil
}

// DetectFile opens the file at path and detects the media type.
func DetectFile(path string) (info Info, err error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, fmt.Errorf("media: open file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("media: close file: %w", cerr)
		}
	}()
	return DetectReader(f)
}

// isText returns true if data appears to be UTF-8 text.
// It checks that the data is valid UTF-8 and has a high ratio of printable
// characters (letters, digits, punctuation, whitespace).
func isText(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	sample := data
	if len(sample) > maxDetectBytes {
		sample = sample[:maxDetectBytes]
	}

	if !utf8.Valid(sample) {
		return false
	}

	printable := 0
	total := 0
	for i := 0; i < len(sample); {
		r, size := utf8.DecodeRune(sample[i:])
		i += size
		total++
		if isPrintableOrWhitespace(r) {
			printable++
		}
	}

	if total == 0 {
		return false
	}

	ratio := float64(printable) / float64(total)
	return ratio >= 0.95
}

func isPrintableOrWhitespace(r rune) bool {
	if r == utf8.RuneError {
		return false
	}
	if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
		return true
	}
	return r >= 0x20 && r != 0x7f
}
