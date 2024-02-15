// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitClient represents a Git client for a repository. The object model uses
// go-git's objects even when the backend is not go-git so that we don't
// redefine it.
type GitClient interface {
	// Commit creates a commit in the repository for a specific Git reference.
	// It accepts a tree ID (hash), the Git reference to create the commit for,
	// a message, and a boolean parameter indicating if the commit must be
	// signed.
	Commit(Hash, string, string, bool) (Hash, error)
	// GetCommit returns the commit object for the supplied ID.
	GetCommit(Hash) (*object.Commit, error)

	// Tag creates a tag in the repository. It accepts the target ID (hash), the
	// name of the tag, a message, and a boolean parameter indicating if the tag
	// object must be signed. Note that a tag reference is also created,
	// pointing to the tag object.
	Tag(Hash, string, string, bool) (Hash, error)
	// GetTag returns the tag object for the supplied ID.
	GetTag(Hash) (*object.Commit, error)

	// GetTree returns the tree object for the supplied ID.
	GetTree(Hash) (*object.Tree, error)

	ReadBlob(Hash) ([]byte, error)
	WriteBlob([]byte) (Hash, error)
	// GetBlob returns the blob object for the supplied ID.
	GetBlob(Hash) (*object.Blob, error)

	// GetReferenceHEAD returns the ID of the tip of the specified Git
	// reference.
	GetReferenceHEAD(string) (Hash, error)
}
