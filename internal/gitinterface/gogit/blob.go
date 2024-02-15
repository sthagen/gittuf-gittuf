// SPDX-License-Identifier: Apache-2.0

package gogit

import (
	"errors"
	"io"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var ErrWrittenBlobLengthMismatch = errors.New("length of blob written does not match length of contents")

// ReadBlob returns the contents of a the blob referenced by blobID.
func (c *GoGitClient) ReadBlob(blobID plumbing.Hash) ([]byte, error) {
	blob, err := c.GetBlob(blobID)
	if err != nil {
		return nil, err
	}

	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(reader)
}

// WriteBlob creates a blob object with the specified contents and returns the
// ID of the resultant blob.
func (c *GoGitClient) WriteBlob(contents []byte) (plumbing.Hash, error) {
	obj := c.repository.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)

	writer, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	length, err := writer.Write(contents)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if length != len(contents) {
		return plumbing.ZeroHash, ErrWrittenBlobLengthMismatch
	}

	return c.repository.Storer.SetEncodedObject(obj)
}

// GetBlob returns the requested blob object.
func (c *GoGitClient) GetBlob(blobID plumbing.Hash) (*object.Blob, error) {
	return c.repository.BlobObject(blobID)
}
