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

var ErrTagAlreadyExists = errors.New("tag already exists")

// Tag creates a new tag in the repository pointing to the specified target.
func (c *GoGitClient) Tag(target plumbing.Hash, name, message string, sign bool) (plumbing.Hash, error) {
	gitConfig, err := signatures.GetGitConfig(c.repository)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	_, err = c.repository.Reference(plumbing.NewTagReferenceName(name), true)
	if err == nil {
		return plumbing.ZeroHash, ErrTagAlreadyExists
	}

	targetObj, err := c.repository.Object(plumbing.AnyObject, target)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	tag := createTagObject(gitConfig, targetObj, name, message, clock)

	if sign {
		signature, err := signatures.SignTag(tag)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		tag.PGPSignature = signature
	}

	return c.ApplyTag(tag)
}

// ApplyTag sets the tag reference after the tag object is written to the
// repository's object store.
func (c *GoGitClient) ApplyTag(tag *object.Tag) (plumbing.Hash, error) {
	tagHash, err := c.WriteTag(tag)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	newRef := plumbing.NewHashReference(plumbing.NewTagReferenceName(tag.Name), tagHash)
	return tagHash, c.repository.Storer.SetReference(newRef)
}

// WriteTag writes the tag to the repository's object store.
func (c *GoGitClient) WriteTag(tag *object.Tag) (plumbing.Hash, error) {
	obj := c.repository.Storer.NewEncodedObject()
	if err := tag.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}

	return c.repository.Storer.SetEncodedObject(obj)
}

// GetTag returns the requested tag object.
func (c *GoGitClient) GetTag(tagID plumbing.Hash) (*object.Tag, error) {
	return c.repository.TagObject(tagID)
}

// createTagObject crafts and returns a new tag object using the specified
// parameters.
func createTagObject(gitConfig *config.Config, targetObj object.Object, name, message string, clock clockwork.Clock) *object.Tag {
	return &object.Tag{
		Name: name,
		Tagger: object.Signature{
			Name:  gitConfig.User.Name,
			Email: gitConfig.User.Email,
			When:  clock.Now(),
		},
		Message:    message,
		TargetType: targetObj.Type(),
		Target:     targetObj.ID(),
	}
}
