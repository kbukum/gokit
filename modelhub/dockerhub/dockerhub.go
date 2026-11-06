package dockerhub

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Defaults and limits.
const (
	DefaultEndpoint         = "https://hub.docker.com"
	DefaultMaxResponseBytes = 2 << 20
	MaxPageSize             = 100
	maxTextBytes            = 256
	maxDescriptionBytes     = 4096
	maxOverviewBytes        = 1 << 20
	maxMediaTypes           = 32
)

// ReasonMalformed marks a Hub response that does not have the documented shape.
const ReasonMalformed = "DOCKERHUB_MALFORMED"

var (
	// namePattern follows the registry's path component grammar.
	namePattern   = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)
	tagPattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// Order sorts a repository listing.
type Order string

// Repository orders.
const (
	// OrderDefault is the Hub's default order.
	OrderDefault Order = ""
	// OrderPulls lists the most pulled first.
	OrderPulls Order = "pull_count"
	// OrderUpdated lists the most recently updated first.
	OrderUpdated Order = "last_updated"
	// OrderName lists by name.
	OrderName Order = "name"
)

// Config configures a [Client].
type Config struct {
	// HTTP sends every request. It must have an endpoint policy granting the Hub API origin, no base URL and no
	// retry policy; the client owns retries.
	HTTP *httpclient.Adapter
	// Clock dates results and drives retry backoff. Nil uses the monotonic system clock.
	Clock util.TimerClock
	// Retry bounds retries of reads. The zero value uses three attempts within 30 seconds.
	Retry resilience.RetryConfig
	// Endpoint is the Hub API origin. Defaults to [DefaultEndpoint].
	Endpoint string
	// MaxResponseBytes bounds a JSON response. Defaults to [DefaultMaxResponseBytes].
	MaxResponseBytes int64
}

// Client reads Docker Hub listings. It is safe for concurrent use.
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
	f, err := fetch.New("docker hub", fetch.Config{HTTP: cfg.HTTP, Clock: cfg.Clock, Retry: cfg.Retry})
	if err != nil {
		return nil, err
	}
	return &Client{fetch: f, endpoint: endpoint, maxBytes: cfg.MaxResponseBytes}, nil
}

// Repository is one repository in a namespace. Counts are the Hub's own; empty fields are not stated.
type Repository struct {
	Namespace, Name string
	// Description is the publisher's short description, often empty.
	Description string
	Stars       int64
	Pulls       int64
	// MediaTypes are the manifest and config media types the repository's tags use.
	MediaTypes []string
	// StorageBytes is the size of every tag together, not of one download.
	StorageBytes int64
	LastUpdated  time.Time
}

// RepositoryDetail is a repository with its overview.
type RepositoryDetail struct {
	Repository
	// Overview is the publisher's long description, untrusted HTML or Markdown.
	Overview string
	ReadAt   time.Time
}

// Tag is one tag of a repository.
type Tag struct {
	Name string
	// Digest is the manifest digest the tag pointed to when it was listed; resolve it to read that artifact.
	Digest string
	// MediaType is the manifest media type.
	MediaType string
	// Size is the total size of the tag's layers as the Hub reports it: the download size, not the size of the
	// weights alone.
	Size       int64
	LastPushed time.Time
}

// Page is one page of a listing.
type Page[T any] struct {
	Items []T
	// Total is the number of items across all pages, as the Hub states it.
	Total int64
	// Next continues the listing, or is 0 on the last page.
	Next int
	// Skipped counts items left out because they did not have the documented shape.
	Skipped int
	ReadAt  time.Time
}

// Query selects a page of a listing. The zero value of each field uses the Hub default.
type Query struct {
	// Text limits repositories to names containing it. Tags ignore it.
	Text string
	// Order sorts repositories. Tags ignore it.
	Order Order
	// PageSize is at most [MaxPageSize].
	PageSize int
	// Page is a [Page.Next] value; 0 is the first page.
	Page int
}

func (q Query) values() (map[string]string, error) {
	switch {
	case len(q.Text) > maxTextBytes:
		return nil, apperr.InvalidInput("text", "must be at most 256 bytes")
	case q.PageSize < 0 || q.PageSize > MaxPageSize:
		return nil, apperr.InvalidInput("page_size", "must be between 0 and 100")
	case q.Page < 0:
		return nil, apperr.InvalidInput("page", "must be a page returned by a listing")
	}
	switch q.Order {
	case OrderDefault, OrderPulls, OrderUpdated, OrderName:
	default:
		return nil, apperr.InvalidInput("order", "is not a supported order")
	}
	v := map[string]string{}
	if q.PageSize > 0 {
		v["page_size"] = strconv.Itoa(q.PageSize)
	}
	if q.Page > 0 {
		v["page"] = strconv.Itoa(q.Page)
	}
	return v, nil
}

type wirePage[T any] struct {
	Count   int64  `json:"count"`
	Next    string `json:"next"`
	Results []T    `json:"results"`
}

type wireRepository struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Description     string   `json:"description"`
	StarCount       int64    `json:"star_count"`
	PullCount       int64    `json:"pull_count"`
	MediaTypes      []string `json:"media_types"`
	StorageSize     int64    `json:"storage_size"`
	LastUpdated     string   `json:"last_updated"`
	FullDescription string   `json:"full_description"`
}

type wireTag struct {
	Name          string `json:"name"`
	Digest        string `json:"digest"`
	MediaType     string `json:"media_type"`
	FullSize      int64  `json:"full_size"`
	TagLastPushed string `json:"tag_last_pushed"`
}

// Repositories lists one page of a namespace's repositories.
func (c *Client) Repositories(ctx context.Context, namespace string, q Query) (*Page[Repository], error) {
	if err := validName("namespace", namespace); err != nil {
		return nil, err
	}
	query, err := q.values()
	if err != nil {
		return nil, err
	}
	if q.Text != "" {
		query["name"] = q.Text
	}
	if q.Order != OrderDefault {
		query["ordering"] = string(q.Order)
	}
	return list(ctx, c, "/v2/namespaces/"+namespace+"/repositories", query, func(w wireRepository) (Repository, bool) {
		r, err := w.repository()
		return r, err == nil && r.Namespace == namespace
	})
}

// Repository reads one repository with its overview.
func (c *Client) Repository(ctx context.Context, namespace, name string) (*RepositoryDetail, error) {
	if err := validName("namespace", namespace); err != nil {
		return nil, err
	}
	if err := validName("name", name); err != nil {
		return nil, err
	}
	resp, err := c.fetch.Get(ctx, fetch.Request{URL: c.endpoint + "/v2/namespaces/" + namespace + "/repositories/" + name, Headers: map[string]string{"Accept": "application/json"}, MaxBytes: c.maxBytes})
	if err != nil {
		return nil, err
	}
	var w wireRepository
	if err = json.Unmarshal(resp.Body, &w); err != nil {
		return nil, malformed("repository")
	}
	r, err := w.repository()
	if err != nil {
		return nil, err
	}
	if r.Namespace != namespace || r.Name != name {
		return nil, malformed("repository.name")
	}
	if len(w.FullDescription) > maxOverviewBytes {
		return nil, malformed("repository.full_description")
	}
	return &RepositoryDetail{Repository: r, Overview: w.FullDescription, ReadAt: c.fetch.Now()}, nil
}

// Tags lists one page of a repository's tags. Query.Text and Query.Order are ignored.
func (c *Client) Tags(ctx context.Context, namespace, name string, q Query) (*Page[Tag], error) {
	if err := validName("namespace", namespace); err != nil {
		return nil, err
	}
	if err := validName("name", name); err != nil {
		return nil, err
	}
	query, err := q.values()
	if err != nil {
		return nil, err
	}
	return list(ctx, c, "/v2/namespaces/"+namespace+"/repositories/"+name+"/tags", query, func(w wireTag) (Tag, bool) {
		if !tagPattern.MatchString(w.Name) || !digestPattern.MatchString(w.Digest) || w.FullSize < 0 || len(w.MediaType) > 255 {
			return Tag{}, false
		}
		return Tag{Name: w.Name, Digest: w.Digest, MediaType: w.MediaType, Size: w.FullSize, LastPushed: timeOf(w.TagLastPushed)}, true
	})
}

func list[W, T any](ctx context.Context, c *Client, path string, query map[string]string, convert func(W) (T, bool)) (*Page[T], error) {
	resp, err := c.fetch.Get(ctx, fetch.Request{URL: c.endpoint + path, Query: query, Headers: map[string]string{"Accept": "application/json"}, MaxBytes: c.maxBytes})
	if err != nil {
		return nil, err
	}
	var wire wirePage[json.RawMessage]
	if err = json.Unmarshal(resp.Body, &wire); err != nil || wire.Count < 0 {
		return nil, malformed("page")
	}
	if len(wire.Results) > MaxPageSize {
		return nil, malformed("results")
	}
	next, err := c.nextPage(wire.Next, path)
	if err != nil {
		return nil, err
	}
	page := &Page[T]{Items: make([]T, 0, len(wire.Results)), Total: wire.Count, Next: next, ReadAt: c.fetch.Now()}
	for _, raw := range wire.Results {
		var w W
		if json.Unmarshal(raw, &w) != nil {
			page.Skipped++
			continue
		}
		item, ok := convert(w)
		if !ok {
			page.Skipped++
			continue
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// nextPage reads the page number from a next link, which must stay on the same origin and path.
func (c *Client) nextPage(next, path string) (int, error) {
	if next == "" {
		return 0, nil
	}
	u, err := url.Parse(next)
	if err != nil || u.Path != path {
		return 0, malformed("next")
	}
	origin, err := security.NormalizeEndpointURL(u.Scheme + "://" + u.Host)
	if err != nil || strings.TrimRight(origin, "/") != c.endpoint {
		return 0, malformed("next")
	}
	n, err := strconv.Atoi(u.Query().Get("page"))
	if err != nil || n < 2 {
		return 0, malformed("next")
	}
	return n, nil
}

func (w wireRepository) repository() (Repository, error) {
	if validName("namespace", w.Namespace) != nil || validName("name", w.Name) != nil {
		return Repository{}, malformed("repository.name")
	}
	if w.StarCount < 0 || w.PullCount < 0 || w.StorageSize < 0 || len(w.Description) > maxDescriptionBytes || len(w.MediaTypes) > maxMediaTypes {
		return Repository{}, malformed("repository")
	}
	for _, m := range w.MediaTypes {
		if len(m) > 255 {
			return Repository{}, malformed("repository.media_types")
		}
	}
	return Repository{
		Namespace: w.Namespace, Name: w.Name, Description: w.Description, Stars: w.StarCount, Pulls: w.PullCount,
		MediaTypes: w.MediaTypes, StorageBytes: w.StorageSize, LastUpdated: timeOf(w.LastUpdated),
	}, nil
}

func validName(field, s string) error {
	if s == "" || len(s) > 255 || !namePattern.MatchString(s) {
		return apperr.InvalidInput(field, "must be a Docker Hub name")
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

func malformed(field string) error {
	return apperr.InvalidFormat(field, "a Docker Hub API response").WithReason(ReasonMalformed)
}
