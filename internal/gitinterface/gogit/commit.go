// SPDX-License-Identifier: Apache-2.0

package gogit

import (
	"errors"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jonboulle/clockwork"
)

// Commit creates a new commit in the repo and sets targetRef's HEAD to the
// commit.
func (c *GoGitClient) Commit(treeHash plumbing.Hash, targetRef string, message string, sign bool) (plumbing.Hash, error) {
	gitConfig, err := getGitConfig(c.repository)
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

	commit := CreateCommitObject(gitConfig, treeHash, []plumbing.Hash{curRef.Hash()}, message, clock)

	if sign {
		signature, err := signCommit(commit)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		commit.PGPSignature = signature
	}

	return c.ApplyCommit(commit, curRef)
}

// CreateCommitObject returns a commit object using the specified parameters.
func CreateCommitObject(gitConfig *config.Config, treeHash plumbing.Hash, parentHashes []plumbing.Hash, message string, clock clockwork.Clock) *object.Commit {
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
