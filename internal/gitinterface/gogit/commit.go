// SPDX-License-Identifier: Apache-2.0

package gogit

import (
	"errors"

	"github.com/gittuf/gittuf/internal/gitinterface/signatures"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jonboulle/clockwork"
)

// Commit creates a new commit in the repo and sets targetRef's HEAD to the
// commit.
func (c *GoGitClient) Commit(treeHash plumbing.Hash, targetRef string, message string, sign bool) (plumbing.Hash, error) {
	gitConfig, err := signatures.GetGitConfig(c.repository)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	targetRefTyped := plumbing.ReferenceName(targetRef)
	curRef, err := c.repository.Reference(targetRefTyped, true)
	if err != nil {
		// FIXME: this is a bit messy
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			// Set empty ref
			if err := c.repository.Storer.SetReference(plumbing.NewHashReference(targetRefTyped, plumbing.ZeroHash)); err != nil {
				return plumbing.ZeroHash, err
			}
			curRef, err = c.repository.Reference(targetRefTyped, true)
			if err != nil {
				return plumbing.ZeroHash, err
			}
		} else {
			return plumbing.ZeroHash, err
		}
	}

	commit := createCommitObject(gitConfig, treeHash, []plumbing.Hash{curRef.Hash()}, message, clock)

	if sign {
		signature, err := signatures.SignCommit(commit)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		commit.PGPSignature = signature
	}

	return c.ApplyCommit(commit, curRef)
}

// ApplyCommit writes a commit object in the repository and updates the
// specified reference to point to the commit.
func (c *GoGitClient) ApplyCommit(commit *object.Commit, curRef *plumbing.Reference) (plumbing.Hash, error) {
	commitHash, err := c.WriteCommit(commit)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	newRef := plumbing.NewHashReference(curRef.Name(), commitHash)
	return commitHash, c.repository.Storer.CheckAndSetReference(newRef, curRef)
}

// WriteCommit stores the commit object in the repository's object store,
// returning the new commit's ID.
func (c *GoGitClient) WriteCommit(commit *object.Commit) (plumbing.Hash, error) {
	obj := c.repository.Storer.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}

	return c.repository.Storer.SetEncodedObject(obj)
}

// GetCommit returns the requested commit object.
func (c *GoGitClient) GetCommit(commitID plumbing.Hash) (*object.Commit, error) {
	return c.repository.CommitObject(commitID)
}

// createCommitObject returns a commit object using the specified parameters.
func createCommitObject(gitConfig *config.Config, treeHash plumbing.Hash, parentHashes []plumbing.Hash, message string, clock clockwork.Clock) *object.Commit {
	author := object.Signature{
		Name:  gitConfig.User.Name,
		Email: gitConfig.User.Email,
		When:  clock.Now(),
	}

	commit := &object.Commit{
		Author:    author,
		Committer: author,
		TreeHash:  treeHash,
		Message:   message,
	}

	if len(parentHashes) > 0 {
		commit.ParentHashes = make([]plumbing.Hash, 0, len(parentHashes))
	}
	for _, parentHash := range parentHashes {
		if !parentHash.IsZero() {
			commit.ParentHashes = append(commit.ParentHashes, parentHash)
		}
	}

	return commit
}
