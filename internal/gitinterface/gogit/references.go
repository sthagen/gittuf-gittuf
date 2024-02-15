// SPDX-License-Identifier: Apache-2.0

package gogit

import "github.com/go-git/go-git/v5/plumbing"

func (c *GoGitClient) GetReferenceHEAD(refPath string) (plumbing.Hash, error) {
	ref, err := c.repository.Reference(plumbing.ReferenceName(refPath), true)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	return ref.Hash(), nil
}
