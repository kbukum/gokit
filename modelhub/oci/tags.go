package oci

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/modelhub/internal/fetch"
)

const maxTagPageBytes = 1 << 20

// Tags lists the tags of a repository.
type Tags struct {
	// Names are the tags, in registry order.
	Names []string
	// Truncated reports that the registry has more tags than the client's MaxTags bound.
	Truncated bool
}

// Tags lists the tags of repo, following pagination up to the configured MaxTags.
func (c *Client) Tags(ctx context.Context, repo string) (Tags, error) {
	if err := validRepository(repo); err != nil {
		return Tags{}, err
	}
	path := "/v2/" + repo + "/tags/list"
	var out Tags
	last := ""
	for {
		query := map[string]string{"n": strconv.Itoa(c.cfg.PageSize)}
		if last != "" {
			query["last"] = last
		}
		resp, err := c.get(ctx, repo, fetch.Request{URL: c.registry + path, Query: query, MaxBytes: maxTagPageBytes})
		if err != nil {
			return Tags{}, err
		}
		var page struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(resp.Body, &page); err != nil {
			return Tags{}, malformed("tags")
		}
		for _, tag := range page.Tags {
			if !tagPattern.MatchString(tag) {
				return Tags{}, malformed("tags")
			}
			if len(out.Names) == c.cfg.MaxTags {
				out.Truncated = true
				return out, nil
			}
			out.Names = append(out.Names, tag)
		}
		next, ok := nextLast(resp.Header("Link"), c.registry, path)
		if !ok {
			return out, nil
		}
		if len(out.Names) == c.cfg.MaxTags {
			out.Truncated = true
			return out, nil
		}
		if next == last || len(page.Tags) == 0 {
			return Tags{}, malformed("link")
		}
		last = next
	}
}

// nextLast reads the "last" cursor of a rel="next" Link that points at the same tags path on the same registry.
func nextLast(link, registry, path string) (string, bool) {
	target, params, ok := strings.Cut(link, ";")
	if !ok || !strings.Contains(strings.ReplaceAll(params, " ", ""), `rel="next"`) {
		return "", false
	}
	u, err := url.Parse(strings.Trim(strings.TrimSpace(target), "<>"))
	if err != nil || u.Path != path {
		return "", false
	}
	if u.Scheme != "" || u.Host != "" {
		if o, err := origin(u.Scheme + "://" + u.Host); err != nil || o != registry {
			return "", false
		}
	}
	last := u.Query().Get("last")
	if !tagPattern.MatchString(last) {
		return "", false
	}
	return last, true
}
