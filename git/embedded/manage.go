package embedded

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	ggconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	rawconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/storer"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// ListBranches lists repository branches matching filter.
func (b *Backend) ListBranches(filter model.BranchFilter) ([]model.Branch, error) {
	cfg, err := b.repo.Config()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	iter, err := b.repo.References()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	defer iter.Close()

	branches := make([]model.Branch, 0)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if !matchesBranchFilter(filter, ref.Name()) {
			return nil
		}
		branch := model.Branch{Name: ref.Name().Short(), Target: oidFromHash(ref.Hash())}
		if ref.Name().IsBranch() {
			branch.Upstream = branchUpstream(cfg, ref.Name().Short())
		}
		branches = append(branches, branch)
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, giterr.Internal(err)
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Name < branches[j].Name })
	return branches, nil
}

// ListTags lists repository tags.
func (b *Backend) ListTags() ([]model.Tag, error) {
	iter, err := b.repo.Tags()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	defer iter.Close()

	tags := make([]model.Tag, 0)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		tag := model.Tag{Name: ref.Name().Short(), Target: oidFromHash(ref.Hash())}
		obj, err := b.repo.TagObject(ref.Hash()) //nolint:govet // inner err shadows outer intentionally
		switch {
		case err == nil:
			tagger := signatureFromObject(obj.Tagger)
			tag.Target = oidFromHash(obj.Target)
			tag.Tagger = &tagger
			tag.Message = obj.Message
		case errors.Is(err, plumbing.ErrObjectNotFound):
		default:
			return err
		}
		tags = append(tags, tag)
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return nil, giterr.Internal(err)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags, nil
}

// CreateBranch creates a local branch at target.
func (b *Backend) CreateBranch(name, target string) error {
	refName := plumbing.NewBranchReferenceName(name)
	if err := refName.Validate(); err != nil {
		return giterr.Internal(err)
	}
	if _, err := b.repo.Reference(refName, false); err == nil {
		return giterr.AlreadyExists("branch", name)
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return giterr.Internal(err)
	}

	hash, err := b.repo.ResolveRevision(plumbing.Revision(target))
	if err != nil {
		return giterr.RefNotFound(target)
	}
	if err := b.repo.Storer.SetReference(plumbing.NewHashReference(refName, *hash)); err != nil {
		return giterr.Internal(err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
func (b *Backend) DeleteBranch(name string) error {
	refName := plumbing.NewBranchReferenceName(name)
	if _, err := b.repo.Reference(refName, false); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return giterr.RefNotFound(name)
		}
		return giterr.Internal(err)
	}
	head, err := b.repo.Head()
	if err == nil && head.Name() == refName {
		return giterr.CheckedOutBranch(name)
	}
	if err := b.repo.Storer.RemoveReference(refName); err != nil {
		return giterr.Internal(err)
	}
	if err := b.repo.DeleteBranch(name); err != nil && !errors.Is(err, gogit.ErrBranchNotFound) {
		return giterr.Internal(err)
	}
	return nil
}

// CreateTag creates a lightweight or annotated tag at target.
func (b *Backend) CreateTag(name, target, message string) error {
	hash, err := b.repo.ResolveRevision(plumbing.Revision(target))
	if err != nil {
		return giterr.RefNotFound(target)
	}
	var opts *gogit.CreateTagOptions
	if message != "" {
		opts = &gogit.CreateTagOptions{Message: message}
	}
	if _, err := b.repo.CreateTag(name, *hash, opts); err != nil {
		if errors.Is(err, gogit.ErrTagExists) {
			return giterr.AlreadyExists("tag", name)
		}
		return giterr.Internal(err)
	}
	return nil
}

// DeleteTag deletes a tag.
func (b *Backend) DeleteTag(name string) error {
	if err := b.repo.DeleteTag(name); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return giterr.RefNotFound(name)
		}
		return giterr.Internal(err)
	}
	return nil
}

// ListRemotes lists configured remotes.
func (b *Backend) ListRemotes() ([]model.Remote, error) {
	cfg, err := b.repo.Config()
	if err != nil {
		return nil, giterr.Internal(err)
	}

	remotes := make([]model.Remote, 0, len(cfg.Remotes))
	for name, remoteCfg := range cfg.Remotes {
		rawRemote, ok := configSubsection(cfg, "remote", name)
		if !ok {
			continue
		}
		fetchSpecs := make([]string, 0, len(remoteCfg.Fetch))
		for _, refspec := range remoteCfg.Fetch {
			fetchSpecs = append(fetchSpecs, refspec.String())
		}
		urls := rawRemote.OptionAll("url")
		url := ""
		if len(urls) > 0 {
			url = urls[0]
		} else if len(remoteCfg.URLs) > 0 {
			url = remoteCfg.URLs[0]
		}
		remotes = append(remotes, model.Remote{
			Name:       name,
			URL:        url,
			FetchSpecs: fetchSpecs,
			PushSpecs:  append([]string(nil), rawRemote.OptionAll("push")...),
		})
	}
	sort.Slice(remotes, func(i, j int) bool { return remotes[i].Name < remotes[j].Name })
	return remotes, nil
}

// Fetch fetches updates from a remote.
func (b *Backend) Fetch(remote string, opts ...model.FetchOption) error {
	options := model.FetchOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	fetchOpts := &gogit.FetchOptions{RemoteName: remote, Prune: options.Prune, Depth: options.Depth}
	if len(options.Refspecs) > 0 {
		fetchOpts.RefSpecs = make([]ggconfig.RefSpec, 0, len(options.Refspecs))
		for _, refspec := range options.Refspecs {
			fetchOpts.RefSpecs = append(fetchOpts.RefSpecs, ggconfig.RefSpec(refspec))
		}
	}
	if err := b.repo.Fetch(fetchOpts); err != nil {
		switch {
		case errors.Is(err, gogit.NoErrAlreadyUpToDate):
			return nil
		case errors.Is(err, gogit.ErrRemoteNotFound):
			return giterr.RemoteNotFound(remote)
		default:
			return giterr.Network(err)
		}
	}
	return nil
}

// Push pushes updates to a remote.
func (b *Backend) Push(remote string, opts ...model.PushOption) error {
	options := model.PushOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	pushOpts := &gogit.PushOptions{RemoteName: remote, Force: options.Force}
	if len(options.Refspecs) > 0 {
		pushOpts.RefSpecs = make([]ggconfig.RefSpec, 0, len(options.Refspecs))
		for _, refspec := range options.Refspecs {
			pushOpts.RefSpecs = append(pushOpts.RefSpecs, ggconfig.RefSpec(refspec))
		}
	}
	if err := b.repo.Push(pushOpts); err != nil {
		switch {
		case errors.Is(err, gogit.NoErrAlreadyUpToDate):
			return nil
		case errors.Is(err, gogit.ErrRemoteNotFound):
			return giterr.RemoteNotFound(remote)
		default:
			return giterr.Network(err)
		}
	}
	return nil
}

// TrackingBranch returns the configured upstream for a local branch.
func (b *Backend) TrackingBranch(branch string) (string, error) {
	refName := plumbing.NewBranchReferenceName(branch)
	if _, err := b.repo.Reference(refName, false); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", giterr.RefNotFound(branch)
		}
		return "", giterr.Internal(err)
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return "", giterr.Internal(err)
	}
	return branchUpstream(cfg, branch), nil
}

// ConfigGet gets the last configured value for a key.
func (b *Backend) ConfigGet(key string) (string, error) {
	values, err := b.ConfigGetAll(key)
	if err != nil {
		return "", err
	}
	return values[len(values)-1], nil
}

// ConfigGetAll gets all configured values for a key.
func (b *Backend) ConfigGetAll(key string) ([]string, error) {
	parts, err := parseConfigKey(key)
	if err != nil {
		return nil, err
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	values := configValues(cfg.Raw, parts.section, parts.subsection, parts.key)
	if len(values) == 0 {
		return nil, giterr.ConfigNotFound(key)
	}
	return values, nil
}

// ConfigSet sets a config key to a single value.
func (b *Backend) ConfigSet(key, value string) error {
	parts, err := parseConfigKey(key)
	if err != nil {
		return err
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return giterr.Internal(err)
	}
	if cfg.Raw == nil {
		cfg.Raw = rawconfig.New()
	}
	cfg.Raw.SetOption(parts.section, parts.subsection, parts.key, value)
	var buf bytes.Buffer
	if err := rawconfig.NewEncoder(&buf).Encode(cfg.Raw); err != nil {
		return giterr.Internal(err)
	}
	updated := ggconfig.NewConfig()
	if err := updated.Unmarshal(buf.Bytes()); err != nil {
		return giterr.Internal(err)
	}
	if err := b.repo.SetConfig(updated); err != nil {
		return giterr.Internal(err)
	}
	return nil
}

type parsedConfigKey struct {
	section    string
	subsection string
	key        string
}

func parseConfigKey(key string) (parsedConfigKey, error) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return parsedConfigKey{}, giterr.InvalidConfigKey(key)
	}
	for _, part := range parts {
		if part == "" {
			return parsedConfigKey{}, giterr.InvalidConfigKey(key)
		}
	}
	parsed := parsedConfigKey{section: parts[0], key: parts[len(parts)-1]}
	if len(parts) > 2 {
		parsed.subsection = strings.Join(parts[1:len(parts)-1], ".")
	}
	return parsed, nil
}

func configValues(raw *rawconfig.Config, section, subsection, key string) []string {
	if raw == nil || !raw.HasSection(section) {
		return nil
	}
	sec := raw.Section(section)
	if subsection == "" {
		return append([]string(nil), sec.OptionAll(key)...)
	}
	if !sec.HasSubsection(subsection) {
		return nil
	}
	return append([]string(nil), sec.Subsection(subsection).OptionAll(key)...)
}

func configSubsection(cfg *ggconfig.Config, section, subsection string) (*rawconfig.Subsection, bool) {
	if cfg == nil || cfg.Raw == nil || !cfg.Raw.HasSection(section) {
		return nil, false
	}
	sec := cfg.Raw.Section(section)
	if !sec.HasSubsection(subsection) {
		return nil, false
	}
	return sec.Subsection(subsection), true
}

func matchesBranchFilter(filter model.BranchFilter, name plumbing.ReferenceName) bool {
	switch filter {
	case model.RemoteBranches:
		return name.IsRemote()
	case model.AllBranches:
		return name.IsBranch() || name.IsRemote()
	default:
		return name.IsBranch()
	}
}

func branchUpstream(cfg *ggconfig.Config, name string) string {
	branchCfg, ok := cfg.Branches[name]
	if !ok || branchCfg.Remote == "" || branchCfg.Merge == "" {
		return ""
	}
	upstream := strings.TrimPrefix(branchCfg.Merge.String(), "refs/heads/")
	if branchCfg.Remote == "." {
		return upstream
	}
	return branchCfg.Remote + "/" + upstream
}
