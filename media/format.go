package media

// Format is a container/codec format identifier (e.g. "mp4", "jpeg", "wav").
//
// It is the light-kit parallel of rskit's media Format enum: a typed handle used across detection [Info], the [FormatInfo] catalog, and the [Registry].
type Format string

// Known formats recognized by the byte-signature detector.
const (
	FormatUnknown Format = ""

	// Image formats.
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatGIF  Format = "gif"
	FormatWebP Format = "webp"
	FormatBMP  Format = "bmp"
	FormatTIFF Format = "tiff"
	FormatICO  Format = "ico"
	FormatAVIF Format = "avif"
	FormatHEIF Format = "heif"

	// Video formats.
	FormatMP4  Format = "mp4"
	FormatMOV  Format = "mov"
	FormatM4V  Format = "m4v"
	FormatWebM Format = "webm"
	FormatMKV  Format = "mkv"
	FormatAVI  Format = "avi"
	FormatFLV  Format = "flv"
	FormatTS   Format = "ts"

	// Audio formats.
	FormatWAV  Format = "wav"
	FormatFLAC Format = "flac"
	FormatOGG  Format = "ogg"
	FormatAAC  Format = "aac"
	FormatMP3  Format = "mp3"
	FormatMIDI Format = "midi"
	FormatAIFF Format = "aiff"
	FormatM4A  Format = "m4a"

	// Text.
	FormatText Format = "txt"
)

// FormatInfo is the static catalog entry describing a known [Format]: its media category, canonical extension, MIME type, and container family.
//
// It parallels rskit's media FormatInfo; the light kit carries only the fields derivable without decoding the payload.
type FormatInfo struct {
	Format    Format `json:"format"`
	Type      Type   `json:"type"`
	Extension string `json:"extension"`
	MimeType  string `json:"mime_type"`
	Container string `json:"container,omitempty"`
}

// knownFormats returns a fresh catalog of every format the detector recognizes. A new slice is returned on each call so no mutable state is shared.
func knownFormats() []FormatInfo {
	return []FormatInfo{
		// Image.
		{FormatJPEG, Image, "jpg", "image/jpeg", ""},
		{FormatPNG, Image, "png", "image/png", ""},
		{FormatGIF, Image, "gif", "image/gif", ""},
		{FormatWebP, Image, "webp", "image/webp", "RIFF"},
		{FormatBMP, Image, "bmp", "image/bmp", ""},
		{FormatTIFF, Image, "tiff", "image/tiff", ""},
		{FormatICO, Image, "ico", "image/x-icon", ""},
		{FormatAVIF, Image, "avif", "image/avif", "ISO BMFF"},
		{FormatHEIF, Image, "heic", "image/heif", "ISO BMFF"},
		// Video.
		{FormatMP4, Video, "mp4", "video/mp4", "ISO BMFF"},
		{FormatMOV, Video, "mov", "video/quicktime", "QuickTime"},
		{FormatM4V, Video, "m4v", "video/x-m4v", "ISO BMFF"},
		{FormatWebM, Video, "webm", "video/webm", "Matroska"},
		{FormatMKV, Video, "mkv", "video/x-matroska", "Matroska"},
		{FormatAVI, Video, "avi", "video/x-msvideo", "RIFF"},
		{FormatFLV, Video, "flv", "video/x-flv", "FLV"},
		{FormatTS, Video, "ts", "video/mp2t", "MPEG-TS"},
		// Audio.
		{FormatWAV, Audio, "wav", "audio/wav", "RIFF"},
		{FormatFLAC, Audio, "flac", "audio/flac", ""},
		{FormatOGG, Audio, "ogg", "audio/ogg", "Ogg"},
		{FormatAAC, Audio, "aac", "audio/aac", ""},
		{FormatMP3, Audio, "mp3", "audio/mpeg", ""},
		{FormatMIDI, Audio, "mid", "audio/midi", ""},
		{FormatAIFF, Audio, "aiff", "audio/aiff", "IFF"},
		{FormatM4A, Audio, "m4a", "audio/mp4", "ISO BMFF"},
		// Text.
		{FormatText, Text, "txt", "text/plain", ""},
	}
}
