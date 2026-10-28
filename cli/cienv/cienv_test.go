package cienv_test

import (
	"maps"
	"testing"

	"github.com/kbukum/gokit/cli/cienv"
)

func env(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok
	}
}

func TestDetect(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		ok   bool
		want map[string]string
	}{
		"github pull request": {
			env: map[string]string{
				"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "acme/app", "GITHUB_SHA": "abc", "GITHUB_RUN_ID": "42",
				"GITHUB_HEAD_REF": "feature", "GITHUB_REF_NAME": "7/merge", "GITHUB_SERVER_URL": "https://github.com", "GITHUB_TOKEN": "secret",
			},
			ok:   true,
			want: map[string]string{"ci": "github-actions", "repository": "acme/app", "commit": "abc", "branch": "feature", "pipeline": "42", "url": "https://github.com/acme/app/actions/runs/42"},
		},
		"github push": {
			env:  map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_REF_NAME": "main"},
			ok:   true,
			want: map[string]string{"ci": "github-actions", "branch": "main"},
		},
		"gitlab": {
			env: map[string]string{
				"GITLAB_CI": "true", "CI": "true", "CI_PROJECT_PATH": "acme/app", "CI_COMMIT_SHA": "def", "CI_COMMIT_REF_NAME": "main",
				"CI_PIPELINE_ID": "7", "CI_PIPELINE_URL": "https://gitlab.com/acme/app/-/pipelines/7", "CI_JOB_TOKEN": "secret",
			},
			ok:   true,
			want: map[string]string{"ci": "gitlab-ci", "repository": "acme/app", "commit": "def", "branch": "main", "pipeline": "7", "url": "https://gitlab.com/acme/app/-/pipelines/7"},
		},
		"unsafe values dropped": {
			env:  map[string]string{"GITLAB_CI": "true", "CI_COMMIT_SHA": "a\nb", "CI_PIPELINE_URL": "javascript:alert(1)", "CI_COMMIT_REF_NAME": string(make([]byte, 300))},
			ok:   true,
			want: map[string]string{"ci": "gitlab-ci"},
		},
		"generic":  {env: map[string]string{"CI": "true"}, ok: true, want: map[string]string{"ci": "ci"}},
		"local":    {env: map[string]string{"HOME": "/home/me"}, want: map[string]string{}},
		"ci false": {env: map[string]string{"CI": "false"}, want: map[string]string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			info, ok := cienv.Detect(env(tc.env))
			if ok != tc.ok || !maps.Equal(info.Labels(), tc.want) {
				t.Fatalf("Detect = %+v, %v; labels %v, want %v", info, ok, info.Labels(), tc.want)
			}
		})
	}
}
