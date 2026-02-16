package pyannote

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

	"github.com/kbukum/gokit/diarization"
	"github.com/kbukum/gokit/provider"
)

const (
	// ProviderName is the registered name for the Pyannote provider.
	ProviderName = "pyannote"

	defaultPyannoteURL     = "http://localhost:8388"
	defaultPyannoteTimeout = 300 * time.Second
)

// Config holds configuration for the Pyannote diarization provider.
type Config struct {
	BaseURL string        `json:"base_url"`
	Timeout time.Duration `json:"timeout"`
}

// Provider implements diarization.Provider using the Pyannote HTTP sidecar.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider creates a new Pyannote diarization provider.
func NewProvider(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultPyannoteURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultPyannoteTimeout
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Factory returns a provider.Factory that creates Pyannote Provider
// instances from a generic config map.
func Factory() provider.Factory[diarization.Provider] {
	return func(cfg map[string]any) (diarization.Provider, error) {
		pc := Config{}
		if v, ok := cfg["base_url"].(string); ok {
			pc.BaseURL = v
		}
		if v, ok := cfg["timeout"].(time.Duration); ok {
			pc.Timeout = v
		}
		return NewProvider(pc), nil
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return ProviderName }

// IsAvailable checks if the Pyannote sidecar is reachable.
func (p *Provider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/health", nil)
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

// Diarize sends audio to the Pyannote sidecar and returns diarization results.
func (p *Provider) Diarize(ctx context.Context, req diarization.DiarizationRequest) (*diarization.DiarizationResponse, error) {
	audioData, err := os.ReadFile(req.AudioPath)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
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

	if req.NumSpeakers > 0 {
		_ = writer.WriteField("num_speakers", fmt.Sprintf("%d", req.NumSpeakers))
	}
	if req.MinSpeakers > 0 {
		_ = writer.WriteField("min_speakers", fmt.Sprintf("%d", req.MinSpeakers))
	}
	if req.MaxSpeakers > 0 {
		_ = writer.WriteField("max_speakers", fmt.Sprintf("%d", req.MaxSpeakers))
	}
	if req.Language != "" {
		_ = writer.WriteField("language", req.Language)
	}
	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/diarize", &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("diarization request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("diarization error (status %d): %s", resp.StatusCode, string(body))
	}

	var result pyannoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode diarization response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("diarization error: %s", result.Error)
	}

	return toDiarizationResponse(&result), nil
}

// --- internal Pyannote API types ---

type pyannoteResponse struct {
	Segments    []pyannoteSegment `json:"segments"`
	NumSpeakers int               `json:"num_speakers"`
	Error       string            `json:"error,omitempty"`
}

type pyannoteSegment struct {
	SpeakerID string  `json:"speaker_id"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Text      string  `json:"text,omitempty"`
}

func toDiarizationResponse(resp *pyannoteResponse) *diarization.DiarizationResponse {
	segments := make([]diarization.Segment, len(resp.Segments))
	for i, seg := range resp.Segments {
		segments[i] = diarization.Segment{
			Speaker: seg.SpeakerID,
			Start:   seg.StartTime,
			End:     seg.EndTime,
			Text:    seg.Text,
		}
	}
	return &diarization.DiarizationResponse{
		Segments:    segments,
		NumSpeakers: resp.NumSpeakers,
	}
}
