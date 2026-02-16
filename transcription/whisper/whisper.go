package whisper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/transcription"
)

const (
	// ProviderName is the registered name for the Whisper provider.
	ProviderName = "whisper"

	defaultWhisperURL     = "http://localhost:8387"
	defaultWhisperModel   = "base"
	defaultWhisperTimeout = 120 * time.Second
)

// Config holds configuration for the Whisper transcription provider.
type Config struct {
	URL         string        `json:"url" yaml:"url"`
	Model       string        `json:"model" yaml:"model"`
	Language    string        `json:"language,omitempty" yaml:"language"`
	Device      string        `json:"device,omitempty" yaml:"device"`
	ComputeType string        `json:"compute_type,omitempty" yaml:"compute_type"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
}

// Provider implements transcription.Provider using a faster-whisper HTTP sidecar.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider creates a new Whisper transcription provider.
func NewProvider(cfg Config) *Provider {
	if cfg.URL == "" {
		cfg.URL = defaultWhisperURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultWhisperModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultWhisperTimeout
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Factory returns a provider.Factory that creates Whisper Provider
// instances from a generic config map.
func Factory() provider.Factory[transcription.Provider] {
	return func(cfg map[string]any) (transcription.Provider, error) {
		wc := Config{}
		if v, ok := cfg["url"].(string); ok {
			wc.URL = v
		}
		if v, ok := cfg["model"].(string); ok {
			wc.Model = v
		}
		if v, ok := cfg["language"].(string); ok {
			wc.Language = v
		}
		if v, ok := cfg["device"].(string); ok {
			wc.Device = v
		}
		if v, ok := cfg["compute_type"].(string); ok {
			wc.ComputeType = v
		}
		if v, ok := cfg["timeout"].(time.Duration); ok {
			wc.Timeout = v
		}
		return NewProvider(wc), nil
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return ProviderName }

// IsAvailable checks if the Whisper sidecar is reachable.
func (p *Provider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.URL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Transcribe sends an audio file to the Whisper sidecar and returns the transcription.
func (p *Provider) Transcribe(ctx context.Context, req transcription.TranscriptionRequest) (*transcription.TranscriptionResponse, error) {
	audioData, err := os.ReadFile(req.AudioPath)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
	}

	model := p.cfg.Model
	if req.Model != "" {
		model = req.Model
	}
	lang := p.cfg.Language
	if req.Language != "" {
		lang = req.Language
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("audio", "audio.wav")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("write audio data: %w", err)
	}

	_ = writer.WriteField("model", model)
	if lang != "" {
		_ = writer.WriteField("language", lang)
	}
	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL+"/transcribe", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("whisper request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper error (status %d): %s", resp.StatusCode, string(body))
	}

	var result whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode whisper response: %w", err)
	}

	return toTranscriptionResponse(&result), nil
}

// --- internal Whisper API response types ---

type whisperResponse struct {
	Text     string           `json:"text"`
	Segments []whisperSegment `json:"segments"`
	Language string           `json:"language"`
}

type whisperSegment struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func toTranscriptionResponse(resp *whisperResponse) *transcription.TranscriptionResponse {
	segments := make([]transcription.Segment, len(resp.Segments))
	for i, seg := range resp.Segments {
		segments[i] = transcription.Segment{
			Start: seg.Start,
			End:   seg.End,
			Text:  seg.Text,
		}
	}

	var duration float64
	if len(resp.Segments) > 0 {
		duration = resp.Segments[len(resp.Segments)-1].End
	}

	return &transcription.TranscriptionResponse{
		Text:     resp.Text,
		Segments: segments,
		Duration: duration,
		Language: resp.Language,
	}
}
