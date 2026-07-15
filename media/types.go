package media

// Type is the broad media category a piece of content belongs to.
//
// It mirrors rskit's media type vocabulary (video/audio/image) and adds the
// light-kit-specific [Text] category used by the byte-signature detector.
type Type int

const (
	// Unknown is returned when the content cannot be classified.
	Unknown Type = iota
	// Video content (e.g. mp4, webm, mkv).
	Video
	// Audio content (e.g. wav, flac, mp3).
	Audio
	// Image content (e.g. jpeg, png, gif).
	Image
	// Text content detected via a UTF-8 printable-ratio heuristic.
	Text
)

// String returns the lowercase human-readable name of the category.
func (t Type) String() string {
	switch t {
	case Video:
		return "video"
	case Audio:
		return "audio"
	case Image:
		return "image"
	case Text:
		return "text"
	default:
		return "unknown"
	}
}

// Info is the result of classifying content, describing the detected category
// and container format without decoding the payload.
type Info struct {
	Type      Type   `json:"type"`
	Format    Format `json:"format"`    // e.g. "mp4", "jpeg", "wav"
	MimeType  string `json:"mime_type"` // e.g. "video/mp4", "image/jpeg"
	Container string `json:"container"` // e.g. "QuickTime", "RIFF", "Matroska"
}
