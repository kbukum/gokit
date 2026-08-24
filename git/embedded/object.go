package embedded

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

func (b *Backend) commitForRef(ref string) (*object.Commit, error) {
	h, err := b.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, giterr.RefNotFound(ref)
	}
	commit, err := b.repo.CommitObject(*h)
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return commit, nil
}

func commitFromObject(commit *object.Commit) model.Commit {
	parents := make([]model.Oid, 0, len(commit.ParentHashes))
	for _, parent := range commit.ParentHashes {
		parents = append(parents, oidFromHash(parent))
	}
	return model.Commit{
		OID:       oidFromHash(commit.Hash),
		Author:    signatureFromObject(commit.Author),
		Committer: signatureFromObject(commit.Committer),
		Message:   commit.Message,
		Parents:   parents,
	}
}

func signatureFromObject(sig object.Signature) model.Signature {
	return model.Signature{Name: sig.Name, Email: sig.Email, When: sig.When}
}
