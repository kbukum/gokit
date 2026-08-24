package embedded

import (
	"context"
	"errors"
	"sort"

	gogit "github.com/go-git/go-git/v5"
	ggconfig "github.com/go-git/go-git/v5/config"
	rawconfig "github.com/go-git/go-git/v5/plumbing/format/config"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

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

// Fetch fetches updates from a remote. The context bounds the remote transfer.
func (b *Backend) Fetch(ctx context.Context, remote string, opts ...model.FetchOption) error {
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
	authMethod, err := transportAuthMethod(b.transport)
	if err != nil {
		return err
	}
	fetchOpts.Auth = authMethod
	if err := b.repo.FetchContext(ctx, fetchOpts); err != nil {
		switch {
		case errors.Is(err, gogit.NoErrAlreadyUpToDate):
			return nil
		case errors.Is(err, gogit.ErrRemoteNotFound):
			return giterr.RemoteNotFound(remote)
		default:
			return mapRemoteError(err)
		}
	}
	return nil
}

// Push pushes updates to a remote. The context bounds the remote transfer.
func (b *Backend) Push(ctx context.Context, remote string, opts ...model.PushOption) error {
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
	pushAuth, err := transportAuthMethod(b.transport)
	if err != nil {
		return err
	}
	pushOpts.Auth = pushAuth
	if err := b.repo.PushContext(ctx, pushOpts); err != nil {
		switch {
		case errors.Is(err, gogit.NoErrAlreadyUpToDate):
			return nil
		case errors.Is(err, gogit.ErrRemoteNotFound):
			return giterr.RemoteNotFound(remote)
		default:
			return mapPushError(err, options.Refspecs)
		}
	}
	return nil
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
