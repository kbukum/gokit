package huggingface

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/gguf"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Defaults and limits.
const (
	DefaultEndpoint         = "https://huggingface.co"
	DefaultMaxResponseBytes = 8 << 20
	DefaultRevision         = "main"
	MaxSearchLimit          = 100
	maxTextBytes            = 256
	maxCursorBytes          = 4096
	maxPathBytes            = 1024
	maxFiles                = 10000
)

// Reasons on [github.com/kbukum/gokit/errors.AppError] values the client returns.
const (
	// ReasonMalformed marks a Hub response that does not have the documented shape.
	ReasonMalformed = "HF_MALFORMED"
	// ReasonAccessRestricted marks a file the caller may not read: the repository is gated or private and the
	// adapter's credentials do not grant access.
	ReasonAccessRestricted = "HF_ACCESS_RESTRICTED"
)

var (
	namePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)
	revisionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	cursorPattern   = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
	shaPattern      = regexp.MustCompile(`^[a-f0-9]{40}$`)
	sha256Pattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Sort orders search results, most first.
type Sort string

// Search orders.
const (
	SortDefault      Sort = ""
	SortDownloads    Sort = "downloads"
	SortLikes        Sort = "likes"
	SortTrending     Sort = "trendingScore"
	SortCreated      Sort = "createdAt"
	SortLastModified Sort = "lastModified"
)

// Gated states whether a repository requires users to accept its terms before downloading.
type Gated string

// Gated values.
const (
	// GatedNone needs no approval.
	GatedNone Gated = "none"
	// GatedAuto is granted automatically once the user accepts the terms.
	GatedAuto Gated = "auto"
	// GatedManual is granted by the repository owner after the user requests access.
	GatedManual Gated = "manual"
	// GatedUnknown is a value the client does not recognize; treat it as gated.
	GatedUnknown Gated = "unknown"
)

// Config configures a [Client].
type Config struct {
	// HTTP sends every request. It must have no base URL and no retry policy; the client owns retries. Its auth, if
	// any, is sent to the Hub for gated and private repositories. It must have an endpoint policy granting the Hub
	// and its file CDN hosts, with EndpointRedirects for file redirects; nothing else is reachable. The Hub picks the
	// CDN host per file and region (seen: cdn-lfs.hf.co, cas-bridge.xethub.hf.co, us.aws.cdn.hf.co); a redirect to
	// a host the policy does not grant fails with a typed forbidden error, never a silent fallback.
	HTTP *httpclient.Adapter
	// Clock dates results and drives retry backoff. Nil uses the monotonic system clock.
	Clock util.TimerClock
	// Retry bounds retries of reads. The zero value uses three attempts within 30 seconds.
	Retry resilience.RetryConfig
	// Endpoint is the Hub origin. Defaults to [DefaultEndpoint].
	Endpoint string
	// MaxResponseBytes bounds a JSON response. Defaults to [DefaultMaxResponseBytes].
	MaxResponseBytes int64
}

// Client reads Hub metadata. It is safe for concurrent use.
type Client struct {
	fetch    *fetch.Client
	endpoint string
	maxBytes int64
}

// New validates cfg and builds a client.
func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	endpoint, err := security.NormalizeEndpointURL(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.Count(endpoint, "/") != 2 {
		return nil, apperr.InvalidInput("endpoint", "An origin without a path is required")
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if cfg.MaxResponseBytes < 0 {
		return nil, apperr.InvalidInput("max_response_bytes", "must be positive")
	}
	f, err := fetch.New("model hub", fetch.Config{HTTP: cfg.HTTP, Clock: cfg.Clock, Retry: cfg.Retry})
	if err != nil {
		return nil, err
	}
	return &Client{fetch: f, endpoint: endpoint, maxBytes: cfg.MaxResponseBytes}, nil
}

// Query filters a model search. The zero value lists models in the Hub's default order.
type Query struct {
	// Text matches model IDs.
	Text string
	// Author limits results to one user or organization.
	Author string
	// Filter limits results to a tag, such as "gguf" or "text-generation".
	Filter string
	// Sort orders results, most first.
	Sort Sort
	// Limit is the page size, at most [MaxSearchLimit]. Zero uses the Hub default.
	Limit int
	// Cursor continues from a previous [Page.Next].
	Cursor string
}

func (q Query) validate() error {
	switch {
	case len(q.Text) > maxTextBytes || len(q.Filter) > maxTextBytes:
		return apperr.InvalidInput("query", "Text and filter must be at most 256 bytes")
	case q.Author != "" && !namePattern.MatchString(q.Author):
		return apperr.InvalidInput("author", "must be a Hub user or organization name")
	case q.Limit < 0 || q.Limit > MaxSearchLimit:
		return apperr.InvalidInput("limit", "must be between 0 and 100")
	case q.Cursor != "" && (len(q.Cursor) > maxCursorBytes || !cursorPattern.MatchString(q.Cursor)):
		return apperr.InvalidInput("cursor", "must be a cursor returned by Search")
	}
	switch q.Sort {
	case SortDefault, SortDownloads, SortLikes, SortTrending, SortCreated, SortLastModified:
		return nil
	default:
		return apperr.InvalidInput("sort", "is not a supported order")
	}
}

// File is one repository file.
type File struct {
	// Path is the file path in the repository.
	Path string
	// Size is the file size in bytes, or 0 when the response did not state it.
	Size int64
	// SHA256 is the LFS content digest, or empty for files outside LFS.
	SHA256 string
}

// Summary is a model as search returns it. Empty fields are not stated by the Hub.
type Summary struct {
	ID, Author string
	// SHA is the commit the summary describes; pin it to read the same files again.
	SHA                     string
	Gated                   Gated
	Private                 bool
	Likes, Downloads        int64
	Tags                    []string
	PipelineTag, Library    string
	Licenses, BaseModels    []string
	CreatedAt, LastModified time.Time
	// Files lists file paths; search results do not include sizes.
	Files []File
}

// Page is one page of search results.
type Page struct {
	Models []Summary
	// Next continues the search, or is empty on the last page.
	Next string
	// Skipped counts listed models left out because they did not have the documented shape.
	Skipped int
	// ReadAt is when the page was read.
	ReadAt time.Time
}

// GGUFInfo holds the GGUF facts the Hub extracts from a repository's GGUF files.
type GGUFInfo struct {
	// Parameters is the parameter count.
	Parameters    int64
	Architecture  string
	ContextLength int64
	// HasChatTemplate reports a chat template; the template itself is not returned.
	HasChatTemplate bool
}

// Model is one repository's details.
type Model struct {
	Summary
	// GGUF holds GGUF facts, or nil.
	GGUF *GGUFInfo
	// Revision is the revision that was read.
	Revision string
	// ReadAt is when the details were read.
	ReadAt time.Time
}

type wireSibling struct {
	RFilename string `json:"rfilename"`
	Size      int64  `json:"size"`
	LFS       *struct {
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	} `json:"lfs"`
}

type wireModel struct {
	ID           string          `json:"id"`
	Author       string          `json:"author"`
	SHA          string          `json:"sha"`
	Gated        json.RawMessage `json:"gated"`
	Private      bool            `json:"private"`
	Likes        int64           `json:"likes"`
	Downloads    int64           `json:"downloads"`
	Tags         []string        `json:"tags"`
	PipelineTag  string          `json:"pipeline_tag"`
	Library      string          `json:"library_name"`
	CreatedAt    string          `json:"createdAt"`
	LastModified string          `json:"lastModified"`
	Siblings     []wireSibling   `json:"siblings"`
	CardData     *struct {
		License   json.RawMessage `json:"license"`
		BaseModel json.RawMessage `json:"base_model"`
	} `json:"cardData"`
	GGUF *struct {
		Total         int64  `json:"total"`
		Architecture  string `json:"architecture"`
		ContextLength int64  `json:"context_length"`
		ChatTemplate  string `json:"chat_template"`
	} `json:"gguf"`
}

// Search lists models matching q.
func (c *Client) Search(ctx context.Context, q Query) (*Page, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	query := map[string]string{"full": "true"}
	for k, v := range map[string]string{"search": q.Text, "author": q.Author, "filter": q.Filter, "sort": string(q.Sort), "cursor": q.Cursor} {
		if v != "" {
			query[k] = v
		}
	}
	if q.Sort != SortDefault {
		query["direction"] = "-1"
	}
	if q.Limit > 0 {
		query["limit"] = strconv.Itoa(q.Limit)
	}
	resp, err := c.fetch.Get(ctx, fetch.Request{URL: c.endpoint + "/api/models", Query: query, MaxBytes: c.maxBytes})
	if err != nil {
		return nil, err
	}
	var wire []json.RawMessage
	if err := json.Unmarshal(resp.Body, &wire); err != nil {
		return nil, malformed("models")
	}
	page := &Page{Models: make([]Summary, 0, len(wire)), Next: c.nextCursor(resp.Header("Link")), ReadAt: c.fetch.Now()}
	for _, raw := range wire {
		var w wireModel
		if json.Unmarshal(raw, &w) != nil {
			page.Skipped++
			continue
		}
		s, err := w.summary()
		if err != nil {
			page.Skipped++
			continue
		}
		page.Models = append(page.Models, s)
	}
	return page, nil
}

// Model reads the details of repo at revision; an empty revision reads [DefaultRevision].
func (c *Client) Model(ctx context.Context, repo, revision string) (*Model, error) {
	if err := validRepo(repo); err != nil {
		return nil, err
	}
	if revision == "" {
		revision = DefaultRevision
	}
	if err := validRevision(revision); err != nil {
		return nil, err
	}
	resp, err := c.fetch.Get(ctx, fetch.Request{
		URL:      c.endpoint + "/api/models/" + repo + "/revision/" + revision,
		Query:    map[string]string{"blobs": "true"},
		MaxBytes: c.maxBytes,
	})
	if err != nil {
		// The Hub answers 401 for repositories that do not exist or that the caller cannot see.
		if fetch.StatusOf(err) == http.StatusUnauthorized {
			return nil, apperr.NotFound("model", repo).WithReason(ReasonAccessRestricted).WithDetail("status", http.StatusUnauthorized)
		}
		return nil, err
	}
	var w wireModel
	if json.Unmarshal(resp.Body, &w) != nil {
		return nil, malformed("model")
	}
	s, err := w.summary()
	if err != nil {
		return nil, err
	}
	m := &Model{Summary: s, Revision: revision, ReadAt: c.fetch.Now()}
	if w.CardData != nil {
		if licenses := stringOrList(w.CardData.License); licenses != nil {
			m.Licenses = licenses
		}
		if bases := stringOrList(w.CardData.BaseModel); bases != nil {
			m.BaseModels = bases
		}
	}
	if g := w.GGUF; g != nil {
		m.GGUF = &GGUFInfo{Parameters: max(g.Total, 0), Architecture: g.Architecture, ContextLength: max(g.ContextLength, 0), HasChatTemplate: g.ChatTemplate != ""}
	}
	return m, nil
}

// File returns a [gguf.Source] that reads a byte prefix of a repository file at revision. Pin revision to a
// commit SHA from [Summary.SHA] to read the file the metadata described.
func (c *Client) File(repo, revision, path string) (gguf.Source, error) {
	if err := validRepo(repo); err != nil {
		return nil, err
	}
	if revision == "" {
		revision = DefaultRevision
	}
	if err := validRevision(revision); err != nil {
		return nil, err
	}
	if err := validPath(path); err != nil {
		return nil, err
	}
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return fileSource{client: c, url: c.endpoint + "/" + repo + "/resolve/" + revision + "/" + strings.Join(segments, "/")}, nil
}

type fileSource struct {
	client *Client
	url    string
}

// OpenPrefix opens up to n bytes of the file.
func (f fileSource) OpenPrefix(ctx context.Context, n int64) (rc io.ReadCloser, err error) {
	rc, _, err = f.client.fetch.Open(ctx, fetch.Request{URL: f.url}, n)
	if status := fetch.StatusOf(err); status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, apperr.Forbidden("The repository is gated or private").WithReason(ReasonAccessRestricted).WithDetail("status", status)
	}
	return rc, err
}

func (w *wireModel) summary() (Summary, error) {
	if err := validRepo(w.ID); err != nil {
		return Summary{}, malformed("id")
	}
	if len(w.Siblings) > maxFiles {
		return Summary{}, malformed("siblings")
	}
	s := Summary{
		ID: w.ID, Author: w.Author, Gated: gatedOf(w.Gated), Private: w.Private, Likes: w.Likes, Downloads: w.Downloads,
		Tags: w.Tags, PipelineTag: w.PipelineTag, Library: w.Library, CreatedAt: timeOf(w.CreatedAt), LastModified: timeOf(w.LastModified),
	}
	if shaPattern.MatchString(w.SHA) {
		s.SHA = w.SHA
	}
	for _, tag := range w.Tags {
		if v, ok := strings.CutPrefix(tag, "license:"); ok {
			s.Licenses = append(s.Licenses, v)
		}
		if v, ok := strings.CutPrefix(tag, "base_model:"); ok && !strings.Contains(v, ":") {
			s.BaseModels = append(s.BaseModels, v)
		}
	}
	for _, sib := range w.Siblings {
		if err := validPath(sib.RFilename); err != nil {
			return Summary{}, malformed("siblings.rfilename")
		}
		f := File{Path: sib.RFilename, Size: max(sib.Size, 0)}
		if sib.LFS != nil && sha256Pattern.MatchString(sib.LFS.SHA256) {
			f.SHA256 = sib.LFS.SHA256
			if sib.LFS.Size > 0 {
				f.Size = sib.LFS.Size
			}
		}
		s.Files = append(s.Files, f)
	}
	return s, nil
}

// nextCursor reads the cursor of a rel="next" Link that points back at the same Hub search.
func (c *Client) nextCursor(link string) string {
	target, params, ok := strings.Cut(link, ";")
	if !ok || !strings.Contains(strings.ReplaceAll(params, " ", ""), `rel="next"`) {
		return ""
	}
	u, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
	if err != nil || u.Path != "/api/models" {
		return ""
	}
	if o, err := security.NormalizeEndpointURL(u.Scheme + "://" + u.Host); err != nil || strings.TrimRight(o, "/") != c.endpoint {
		return ""
	}
	cursor := u.Query().Get("cursor")
	if len(cursor) > maxCursorBytes || !cursorPattern.MatchString(cursor) {
		return ""
	}
	return cursor
}

func gatedOf(raw json.RawMessage) Gated {
	switch strings.TrimSpace(string(raw)) {
	case "", "false", "null":
		return GatedNone
	case `"auto"`:
		return GatedAuto
	case `"manual"`:
		return GatedManual
	default:
		return GatedUnknown
	}
}

func stringOrList(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil && one != "" {
		return []string{one}
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return list
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

func validRepo(repo string) error {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || !namePattern.MatchString(owner) || !namePattern.MatchString(name) || strings.Contains(repo, "..") {
		return apperr.InvalidInput("repo", "must be an owner/name Hub repository ID")
	}
	return nil
}

func validRevision(rev string) error {
	if !revisionPattern.MatchString(rev) || strings.Contains(rev, "..") {
		return apperr.InvalidInput("revision", "must be a branch, tag or commit SHA")
	}
	return nil
}

func validPath(p string) error {
	if p == "" || len(p) > maxPathBytes || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") {
		return apperr.InvalidInput("path", "must be a relative repository file path")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return apperr.InvalidInput("path", "must be a relative repository file path")
		}
	}
	return nil
}

func malformed(field string) error {
	return apperr.InvalidFormat(field, "a Hub API response").WithReason(ReasonMalformed)
}
