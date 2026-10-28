package cienv

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// Provider names a CI system.
type Provider string

const (
	// GitHubActions is GitHub Actions.
	GitHubActions Provider = "github-actions"
	// GitLabCI is GitLab CI/CD.
	GitLabCI Provider = "gitlab-ci"
	// Generic is a CI system that sets only CI=true.
	Generic Provider = "ci"
)

// Info is what the CI system says about the current build. Empty fields were not set.
type Info struct {
	Provider   Provider
	Repository string
	Commit     string
	Branch     string
	// Pipeline is the run or pipeline id.
	Pipeline string
	// URL links to the run or pipeline; it is empty unless it is an http or https URL.
	URL string
}

// MaxValue bounds each value; longer values are dropped rather than cut.
const MaxValue = 256

// Detect returns the CI build the environment describes, and false outside CI.
func Detect(lookup func(string) (string, bool)) (Info, bool) {
	get := func(name string) string {
		v, ok := lookup(name)
		if !ok || len(v) > MaxValue || !utf8.ValidString(v) || strings.ContainsFunc(v, isControl) {
			return ""
		}
		return strings.TrimSpace(v)
	}
	switch {
	case get("GITHUB_ACTIONS") == "true":
		info := Info{Provider: GitHubActions, Repository: get("GITHUB_REPOSITORY"), Commit: get("GITHUB_SHA"), Pipeline: get("GITHUB_RUN_ID")}
		info.Branch = get("GITHUB_HEAD_REF")
		if info.Branch == "" {
			info.Branch = get("GITHUB_REF_NAME")
		}
		if server, repo := get("GITHUB_SERVER_URL"), info.Repository; server != "" && repo != "" && info.Pipeline != "" {
			info.URL = webURL(server + "/" + repo + "/actions/runs/" + info.Pipeline)
		}
		return info, true
	case get("GITLAB_CI") == "true":
		info := Info{Provider: GitLabCI, Repository: get("CI_PROJECT_PATH"), Commit: get("CI_COMMIT_SHA"), Pipeline: get("CI_PIPELINE_ID"), URL: webURL(get("CI_PIPELINE_URL"))}
		info.Branch = get("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
		if info.Branch == "" {
			info.Branch = get("CI_COMMIT_REF_NAME")
		}
		return info, true
	case get("CI") == "true" || get("CI") == "1":
		return Info{Provider: Generic}, true
	}
	return Info{}, false
}

// Labels returns the set fields as labels: ci, repository, commit, branch, pipeline and url.
func (i Info) Labels() map[string]string {
	out := map[string]string{}
	for key, value := range map[string]string{
		"ci": string(i.Provider), "repository": i.Repository, "commit": i.Commit, "branch": i.Branch, "pipeline": i.Pipeline, "url": i.URL,
	} {
		if value != "" {
			out[key] = value
		}
	}
	return out
}

func webURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}
