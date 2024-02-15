// SPDX-License-Identifier: Apache-2.0

package gogit

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GetTree returns the requested tree object.
func (c *GoGitClient) GetTree(treeID plumbing.Hash) (*object.Tree, error) {
	return c.repository.TreeObject(treeID)
}
