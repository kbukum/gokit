package embedded

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
	"github.com/kbukum/gokit/git/internal/model"
)

// TreeHash returns the OID of the tree at the given path and revision.
func (b *Backend) TreeHash(revision, path string) (model.TreeHash, error) {
	tree, err := b.resolveTree(revision, path)
	if err != nil {
		return model.Oid{}, err
	}
	return oidFromHash(tree.Hash), nil
}

// FileAt returns the content of a file at the given revision and path.
func (b *Backend) FileAt(revision, path string) ([]byte, error) {
	file, _, err := b.fileAt(revision, path)
	if err != nil {
		return nil, err
	}

	content, err := file.Contents()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return []byte(content), nil
}

// FileAtBounded returns file content only when the blob fits maxBytes.
func (b *Backend) FileAtBounded(revision, path string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, giterr.InvalidArg("maxBytes", "maxBytes must be non-negative")
	}
	file, normalized, err := b.fileAt(revision, path)
	if err != nil {
		return nil, err
	}
	if file.Size > maxBytes {
		return nil, giterr.FileTooLarge(normalized, revision, file.Size, maxBytes)
	}
	content, err := file.Contents()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	return []byte(content), nil
}

func (b *Backend) fileAt(revision, path string) (*object.File, string, error) {
	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, "", err
	}

	path = strings.ReplaceAll(path, "\\", "/")
	file, err := commit.File(path)
	if err != nil {
		return nil, "", giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}
	return file, path, nil
}

// ListEntries returns the entries in a tree at the given revision and path.
func (b *Backend) ListEntries(revision, path string) ([]model.TreeEntry, error) {
	tree, err := b.resolveTree(revision, path)
	if err != nil {
		return nil, err
	}

	entries := make([]model.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		entries = append(entries, model.TreeEntry{
			Name:     entry.Name,
			OID:      oidFromHash(entry.Hash),
			Kind:     entryKindFromMode(entry.Mode),
			Filemode: uint32(entry.Mode),
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (b *Backend) resolveTree(revision, path string) (*object.Tree, error) {
	commit, err := b.commitForRef(revision)
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if path == "" || path == "." || path == "/" {
		return tree, nil
	}
	subtree, err := tree.Tree(path)
	if err != nil {
		return nil, giterr.RefNotFound(fmt.Sprintf("%s:%s", revision, path))
	}
	return subtree, nil
}

func entryKindFromMode(m filemode.FileMode) model.EntryKind {
	switch m {
	case filemode.Dir:
		return model.EntryKindTree
	case filemode.Submodule:
		return model.EntryKindSubmodule
	default:
		return model.EntryKindBlob
	}
}
