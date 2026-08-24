package embedded

import (
	"errors"
	"sort"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

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
			return giterr.Internal(err)
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

// CreateSignedTag is unsupported by go-git; Repo routes signed tags to the CLI backend.
func (b *Backend) CreateSignedTag(name, target, message string, opts model.SignOptions) error {
	return giterr.SigningNotSupported()
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
