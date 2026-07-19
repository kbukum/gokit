package model

import (
	"strings"
	"testing"
	"time"

	gitauth "github.com/kbukum/gokit/git/auth"
)

func TestWithExtraArgsAppends(t *testing.T) {
	t.Parallel()

	opts := ApplyOptions(
		WithExtraArgs("--first"),
		WithExtraArgs("--second", "--third"),
	)

	want := []string{"--first", "--second", "--third"}
	if len(opts.ExtraArgs) != len(want) {
		t.Fatalf("ExtraArgs len = %d, want %d", len(opts.ExtraArgs), len(want))
	}
	for i := range want {
		if opts.ExtraArgs[i] != want[i] {
			t.Fatalf("ExtraArgs[%d] = %q, want %q", i, opts.ExtraArgs[i], want[i])
		}
	}
}

func TestOpenOptions(t *testing.T) {
	t.Parallel()

	transport := gitauth.Token{Username: "user", Value: "token"}
	signing := gitauth.GPGSign{KeyID: "key"}
	opts := ApplyOptions(
		WithPreferCLI(true),
		WithCLIPath("/usr/bin/git"),
		WithTransport(transport),
		WithSigning(signing),
		nil,
	)

	if !opts.PreferCLI || opts.CLIPath != "/usr/bin/git" {
		t.Fatalf("open options = %+v", opts)
	}
	if opts.Transport != transport {
		t.Fatalf("transport = %#v, want %#v", opts.Transport, transport)
	}
	if opts.Signing != signing {
		t.Fatalf("signing = %#v, want %#v", opts.Signing, signing)
	}
}

func TestOptionFunctionsPopulateConfigs(t *testing.T) {
	t.Parallel()

	diff := DiffOptions{}
	WithDiffContext(7)(&diff)
	WithDiffNameOnly(true)(&diff)
	WithDiffExtraArgs("--stat")(&diff)
	if diff.ContextLines != 7 || !diff.NameOnly || diff.ExtraArgs[0] != "--stat" {
		t.Fatalf("diff options = %+v", diff)
	}

	log := LogOptions{}
	WithLogExtraArgs("--first-parent")(&log)
	if log.ExtraArgs[0] != "--first-parent" {
		t.Fatalf("log options = %+v", log)
	}

	blame := BlameOptions{}
	WithLineRange(2, 4)(&blame)
	WithIgnoreWhitespace(true)(&blame)
	WithBlameExtraArgs("--contents", "file")(&blame)
	if blame.StartLine != 2 || blame.EndLine != 4 || !blame.IgnoreWhitespace || len(blame.ExtraArgs) != 2 {
		t.Fatalf("blame options = %+v", blame)
	}

	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	author := Signature{Name: "Author", Email: "author@example.com", When: when}
	committer := Signature{Name: "Committer", Email: "committer@example.com", When: when}
	commit := CommitOptions{}
	WithCommitAuthor(author)(&commit)
	WithCommitCommitter(committer)(&commit)
	WithCommitSign(true)(&commit)
	WithCommitAmend(true)(&commit)
	WithCommitExtraArgs("--allow-empty")(&commit)
	author.Name = "mutated"
	if commit.Author.Name != "Author" || commit.Committer.Name != "Committer" || !commit.Sign || !commit.Amend || commit.ExtraArgs[0] != "--allow-empty" {
		t.Fatalf("commit options = %+v", commit)
	}

	fetch := FetchOptions{}
	WithFetchPrune(true)(&fetch)
	WithFetchDepth(3)(&fetch)
	WithFetchRefspecs("a", "b")(&fetch)
	WithFetchExtraArgs("--tags")(&fetch)
	if !fetch.Prune || fetch.Depth != 3 || len(fetch.Refspecs) != 2 || fetch.ExtraArgs[0] != "--tags" {
		t.Fatalf("fetch options = %+v", fetch)
	}

	push := PushOptions{}
	WithPushForce(true)(&push)
	WithPushRefspecs("main")(&push)
	WithPushExtraArgs("--atomic")(&push)
	if !push.Force || push.Refspecs[0] != "main" || push.ExtraArgs[0] != "--atomic" {
		t.Fatalf("push options = %+v", push)
	}

	describe := DescribeOptions{}
	WithDescribeMatch("v*")(&describe)
	WithDescribeAnnotatedTagsOnly(true)(&describe)
	WithDescribeLong(true)(&describe)
	WithDescribeAbbreviated(true)(&describe)
	WithDescribeAlways(true)(&describe)
	WithDescribeExtraArgs("--dirty")(&describe)
	if describe.Match != "v*" || !describe.AnnotatedTagsOnly || !describe.Long || !describe.Abbreviated || !describe.Always || describe.ExtraArgs[0] != "--dirty" {
		t.Fatalf("describe options = %+v", describe)
	}

	grep := GrepOptions{}
	WithGrepPathspecs("*.go")(&grep)
	WithGrepIgnoreCase(true)(&grep)
	WithGrepLineNumbers(true)(&grep)
	WithGrepExtraArgs("--fixed-strings")(&grep)
	if grep.Pathspecs[0] != "*.go" || !grep.IgnoreCase || !grep.LineNumbers || grep.ExtraArgs[0] != "--fixed-strings" {
		t.Fatalf("grep options = %+v", grep)
	}

	merge := MergeOptions{}
	WithMergeCommit(true)(&merge)
	WithMergeFFOnly(true)(&merge)
	WithMergeNoFastForward(true)(&merge)
	WithMergeSquash(true)(&merge)
	WithMergeMessage("merge")(&merge)
	WithMergeExtraArgs("--verify")(&merge)
	if !merge.Commit || !merge.FFOnly || !merge.NoFastForward || !merge.Squash || merge.Message != "merge" || merge.ExtraArgs[0] != "--verify" {
		t.Fatalf("merge options = %+v", merge)
	}

	rebase := RebaseOptions{}
	WithRebaseInteractive(true)(&rebase)
	WithRebaseAutosquash(true)(&rebase)
	WithRebaseExtraArgs("--rebase-merges")(&rebase)
	if !rebase.Interactive || !rebase.Autosquash || rebase.ExtraArgs[0] != "--rebase-merges" {
		t.Fatalf("rebase options = %+v", rebase)
	}

	cherryPick := CherryPickOptions{}
	WithCherryPickMainline(2)(&cherryPick)
	WithCherryPickNoCommit(true)(&cherryPick)
	WithCherryPickExtraArgs("--strategy", "ours")(&cherryPick)
	if cherryPick.Mainline != 2 || !cherryPick.NoCommit || len(cherryPick.ExtraArgs) != 2 {
		t.Fatalf("cherry-pick options = %+v", cherryPick)
	}

	checkout := CheckoutOptions{}
	WithCheckoutCreateBranch("feature")(&checkout)
	WithCheckoutForce(true)(&checkout)
	WithCheckoutDetach(true)(&checkout)
	WithCheckoutExtraArgs("--conflict", "diff3")(&checkout)
	if checkout.CreateBranch != "feature" || !checkout.Force || !checkout.Detach || len(checkout.ExtraArgs) != 2 {
		t.Fatalf("checkout options = %+v", checkout)
	}

	clean := CleanOptions{}
	WithCleanDirectories(true)(&clean)
	WithCleanIgnored(true)(&clean)
	WithCleanForce(true)(&clean)
	WithCleanExtraArgs("--quiet")(&clean)
	if !clean.Directories || !clean.Ignored || !clean.Force || clean.ExtraArgs[0] != "--quiet" {
		t.Fatalf("clean options = %+v", clean)
	}
}

func FuzzOidString(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, input []byte) {
		var oid Oid
		copy(oid[:], input)
		got := oid.String()
		if len(got) != 40 {
			t.Fatalf("OID string length = %d, want 40", len(got))
		}
		for _, r := range got {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Fatalf("OID string contains non-hex rune %q in %q", r, got)
			}
		}
	})
}
