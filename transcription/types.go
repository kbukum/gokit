package transcription

// TranscriptionRequest holds parameters for a transcription call.
type TranscriptionRequest struct {
	// AudioPath is the path to the audio file to transcribe.
	AudioPath string `json:"audio_path"`
	// Language is the expected language of the audio (e.g. "en").
	Language string `json:"language,omitempty"`
	// Model is the transcription model to use.
	Model string `json:"model,omitempty"`
	// Format is the desired output format (e.g. "text", "json", "srt").
	Format string `json:"format,omitempty"`
}

// TranscriptionResponse holds the result of a transcription call.
type TranscriptionResponse struct {
	// Text is the full transcription text.
	Text string `json:"text"`
	// Segments contains time-aligned transcript segments.
	Segments []Segment `json:"segments,omitempty"`
	// Duration is the audio duration in seconds.
	Duration float64 `json:"duration,omitempty"`
	// Language is the detected or specified language.
	Language string `json:"language,omitempty"`
}

// Segment represents a time-aligned portion of a transcript.
type Segment struct {
	// Start is the segment start time in seconds.
	Start float64 `json:"start"`
	// End is the segment end time in seconds.
	End float64 `json:"end"`
	// Text is the transcribed text for this segment.
	Text string `json:"text"`
	// Speaker is the identified speaker label, if available.
	Speaker string `json:"speaker,omitempty"`
}
