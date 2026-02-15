package diarization

// DiarizationRequest holds parameters for a diarization call.
type DiarizationRequest struct {
	// AudioPath is the path to the audio file to diarize.
	AudioPath string `json:"audio_path"`
	// NumSpeakers is the exact number of speakers (0 = auto-detect).
	NumSpeakers int `json:"num_speakers,omitempty"`
	// MinSpeakers is the minimum expected number of speakers.
	MinSpeakers int `json:"min_speakers,omitempty"`
	// MaxSpeakers is the maximum expected number of speakers.
	MaxSpeakers int `json:"max_speakers,omitempty"`
	// Language is the expected language of the audio (e.g. "en").
	Language string `json:"language,omitempty"`
}

// DiarizationResponse holds the result of a diarization call.
type DiarizationResponse struct {
	// Segments contains speaker-attributed time segments.
	Segments []Segment `json:"segments"`
	// NumSpeakers is the number of speakers detected.
	NumSpeakers int `json:"num_speakers"`
}

// Segment represents a speaker-attributed time range.
type Segment struct {
	// Speaker is the identified speaker label.
	Speaker string `json:"speaker"`
	// Start is the segment start time in seconds.
	Start float64 `json:"start"`
	// End is the segment end time in seconds.
	End float64 `json:"end"`
	// Text is the transcribed text for this segment, if available.
	Text string `json:"text,omitempty"`
}
