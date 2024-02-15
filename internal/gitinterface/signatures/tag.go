// SPDX-License-Identifier: Apache-2.0

package signatures

import (
	"context"
	"errors"
	"io"

	"github.com/gittuf/gittuf/internal/signerverifier"
	"github.com/gittuf/gittuf/internal/tuf"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

func SignTag(tag *object.Tag) (string, error) {
	tagContents, err := getTagBytesWithoutSignature(tag)
	if err != nil {
		return "", err
	}

	return SignGitObject(tagContents)
}

// VerifyTagSignature is used to verify a cryptographic signature associated
// with tag using TUF public keys.
func VerifyTagSignature(ctx context.Context, tag *object.Tag, key *tuf.Key) error {
	switch key.KeyType {
	case signerverifier.GPGKeyType:
		if _, err := tag.Verify(key.KeyVal.Public); err != nil {
			return ErrIncorrectVerificationKey
		}

		return nil
	case signerverifier.RSAKeyType, signerverifier.ECDSAKeyType, signerverifier.ED25519KeyType:
		tagContents, err := getTagBytesWithoutSignature(tag)
		if err != nil {
			return errors.Join(ErrVerifyingSSHSignature, err)
		}
		tagSignature := []byte(tag.PGPSignature)

		if err := VerifySSHKeySignature(key, tagContents, tagSignature); err != nil {
			return errors.Join(ErrIncorrectVerificationKey, err)
		}

		return nil
	case signerverifier.FulcioKeyType:
		tagContents, err := getTagBytesWithoutSignature(tag)
		if err != nil {
			return errors.Join(ErrVerifyingSigstoreSignature, err)
		}
		tagSignature := []byte(tag.PGPSignature)

		if err := VerifyGitsignSignature(ctx, key, tagContents, tagSignature); err != nil {
			return errors.Join(ErrIncorrectVerificationKey, err)
		}

		return nil
	}

	return ErrUnknownSigningMethod
}

func getTagBytesWithoutSignature(tag *object.Tag) ([]byte, error) {
	tagEncoded := memory.NewStorage().NewEncodedObject()
	if err := tag.EncodeWithoutSignature(tagEncoded); err != nil {
		return nil, err
	}
	r, err := tagEncoded.Reader()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(r)
}
