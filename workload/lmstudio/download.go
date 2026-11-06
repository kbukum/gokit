package lmstudio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
	"unicode"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

const (
	maxModelBytes        = 2048
	maxQuantizationBytes = 64
)

var jobIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// DownloadStatus is the state of a download job.
type DownloadStatus string

// Download states LM Studio reports.
const (
	DownloadDownloading       DownloadStatus = "downloading"
	DownloadPaused            DownloadStatus = "paused"
	DownloadCompleted         DownloadStatus = "completed"
	DownloadFailed            DownloadStatus = "failed"
	DownloadAlreadyDownloaded DownloadStatus = "already_downloaded"
)

// Terminal reports whether the job has stopped changing.
func (s DownloadStatus) Terminal() bool {
	return s == DownloadCompleted || s == DownloadFailed || s == DownloadAlreadyDownloaded
}

func (s DownloadStatus) known() bool {
	return s == DownloadDownloading || s == DownloadPaused || s.Terminal()
}

// DownloadJob is a download job's state. Zero fields are not stated by LM Studio.
type DownloadJob struct {
	// JobID identifies the job; empty when the model was already downloaded.
	JobID  string
	Status DownloadStatus
	// DownloadedBytes and TotalBytes report progress.
	DownloadedBytes, TotalBytes int64
	// BytesPerSecond is the current speed while downloading.
	BytesPerSecond float64
	// StartedAt, EstimatedCompletion and CompletedAt are the job's times.
	StartedAt, EstimatedCompletion, CompletedAt time.Time
}

// DownloadFailedError reports a download job that LM Studio marked failed. LM Studio states no cause.
type DownloadFailedError struct {
	JobID string
}

func (e *DownloadFailedError) Error() string { return "lmstudio: download job " + e.JobID + " failed" }

// Downloader starts model downloads on an LM Studio server and follows their progress.
type Downloader struct {
	http  *httpclient.Adapter
	base  string
	poll  time.Duration
	clock util.TimerClock
}

// Downloader does not implement workload.ModelPuller: that contract stops the pull when ctx ends, and LM Studio has
// no API to cancel a download.

// NewDownloader builds a downloader from a validated copy of cfg.
func NewDownloader(cfg Config) (*Downloader, error) {
	c, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	hc, err := c.adapter()
	if err != nil {
		return nil, err
	}
	return &Downloader{http: hc, base: c.BaseURL, poll: c.PollInterval, clock: c.Clock}, nil
}

type wireJob struct {
	JobID               string   `json:"job_id"`
	Status              string   `json:"status"`
	DownloadedBytes     *int64   `json:"downloaded_bytes"`
	TotalSizeBytes      *int64   `json:"total_size_bytes"`
	BytesPerSecond      *float64 `json:"bytes_per_second"`
	StartedAt           string   `json:"started_at"`
	EstimatedCompletion string   `json:"estimated_completion"`
	CompletedAt         string   `json:"completed_at"`
}

// Start asks LM Studio to download model: a catalog identifier, such as "ibm/granite-4-micro", or a Hugging Face
// repository URL. quantization, such as "Q4_K_M", selects a file and applies only to Hugging Face URLs. Start is not
// retried, because a repeated request could start a second download.
func (d *Downloader) Start(ctx context.Context, model, quantization string) (*DownloadJob, error) {
	if err := validText("model", model, maxModelBytes, true); err != nil {
		return nil, err
	}
	if err := validText("quantization", quantization, maxQuantizationBytes, false); err != nil {
		return nil, err
	}
	body := map[string]string{"model": model}
	if quantization != "" {
		body["quantization"] = quantization
	}
	resp, err := d.http.Do(ctx, httpclient.Request{
		Method:  http.MethodPost,
		Path:    d.base + "/api/v1/models/download",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	})
	if err != nil {
		return nil, requestError("start download", resp, err)
	}
	return decodeJob(resp.Body, "")
}

// Status reads the state of a download job. Transient failures are retried on the configured clock.
func (d *Downloader) Status(ctx context.Context, jobID string) (*DownloadJob, error) {
	if !jobIDPattern.MatchString(jobID) {
		return nil, apperr.InvalidInput("job_id", "must be a job ID returned by Start")
	}
	retry := resilience.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		MaxElapsedTime: 10 * time.Second,
		Strategy:       resilience.ExponentialBackoff,
		BackoffFactor:  2,
		RetryIf:        transient,
		Clock:          d.clock,
	}
	body, err := resilience.Retry(ctx, retry, func() ([]byte, error) {
		resp, err := d.http.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: d.base + "/api/v1/models/download/status/" + jobID})
		if err != nil {
			return nil, requestError("download status", resp, err)
		}
		return resp.Body, nil
	})
	if err != nil {
		if apiErr, ok := errors.AsType[*Error](err); ok {
			return nil, apiErr
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return decodeJob(body, jobID)
}

// Follow polls a download job every PollInterval until it reaches a terminal status or ctx ends, calling onProgress,
// if set, with each state it reads. It returns the terminal state, or a [*DownloadFailedError] when the job failed.
// Bound a long download with ctx; canceling ctx stops following, not the download itself.
func (d *Downloader) Follow(ctx context.Context, jobID string, onProgress func(DownloadJob)) (*DownloadJob, error) {
	return d.follow(ctx, jobID, func(j DownloadJob) error {
		if onProgress != nil {
			onProgress(j)
		}
		return nil
	})
}

func (d *Downloader) follow(ctx context.Context, jobID string, onProgress func(DownloadJob) error) (*DownloadJob, error) {
	for {
		job, err := d.Status(ctx, jobID)
		if err != nil {
			return nil, err
		}
		if err := onProgress(*job); err != nil {
			return nil, err
		}
		switch job.Status {
		case DownloadFailed:
			return job, &DownloadFailedError{JobID: jobID}
		case DownloadCompleted, DownloadAlreadyDownloaded:
			return job, nil
		case DownloadDownloading, DownloadPaused:
		}
		timer := d.clock.NewTimer(d.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C():
		}
	}
}

func decodeJob(body []byte, wantID string) (*DownloadJob, error) {
	refuse := apperr.InvalidFormat("download_job", "an LM Studio download job")
	if len(body) > maxBody {
		return nil, refuse
	}
	var w wireJob
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, refuse
	}
	job := &DownloadJob{JobID: w.JobID, Status: DownloadStatus(w.Status)}
	if !job.Status.known() || (w.JobID != "" && !jobIDPattern.MatchString(w.JobID)) || (wantID != "" && w.JobID != "" && w.JobID != wantID) {
		return nil, refuse
	}
	if job.JobID == "" && job.Status != DownloadAlreadyDownloaded {
		if wantID == "" {
			return nil, refuse
		}
		job.JobID = wantID
	}
	for _, n := range []*int64{w.DownloadedBytes, w.TotalSizeBytes} {
		if n != nil && *n < 0 {
			return nil, refuse
		}
	}
	if w.BytesPerSecond != nil && *w.BytesPerSecond < 0 {
		return nil, refuse
	}
	if w.DownloadedBytes != nil {
		job.DownloadedBytes = *w.DownloadedBytes
	}
	if w.TotalSizeBytes != nil {
		job.TotalBytes = *w.TotalSizeBytes
	}
	if w.BytesPerSecond != nil {
		job.BytesPerSecond = *w.BytesPerSecond
	}
	job.StartedAt, job.EstimatedCompletion, job.CompletedAt = timeOf(w.StartedAt), timeOf(w.EstimatedCompletion), timeOf(w.CompletedAt)
	return job, nil
}

func requestError(op string, resp *httpclient.Response, err error) error {
	if resp != nil {
		return &Error{Status: resp.StatusCode}
	}
	return fmt.Errorf("lmstudio: %s: %w", op, err)
}

// transient reports a failure worth retrying: a transport failure or a throttled or unavailable server.
func transient(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= http.StatusInternalServerError
	}
	// Policy refusals and refused redirects are typed and final; untyped transport failures are worth a retry.
	if app, ok := apperr.AsAppError(err); ok {
		return app.Retryable
	}
	return true
}

func validText(field, s string, maxBytes int, required bool) error {
	if (required && s == "") || len(s) > maxBytes {
		return apperr.InvalidInput(field, "has an invalid length")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return apperr.InvalidInput(field, "must not contain control characters")
		}
	}
	return nil
}

func timeOf(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
