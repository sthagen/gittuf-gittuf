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

func SignCommit(commit *object.Commit) (string, error) {
	commitContents, err := getCommitBytesWithoutSignature(commit)
	if err != nil {
		return "", err
	}

	return SignGitObject(commitContents)
}

// VerifyCommitSignature is used to verify a cryptographic signature associated
// with commit using TUF public keys.
func VerifyCommitSignature(ctx context.Context, commit *object.Commit, key *tuf.Key) error {
	switch key.KeyType {
	case signerverifier.GPGKeyType:
		if _, err := commit.Verify(key.KeyVal.Public); err != nil {
			return ErrIncorrectVerificationKey
		}

		return nil
	case signerverifier.RSAKeyType, signerverifier.ECDSAKeyType, signerverifier.ED25519KeyType:
		commitContents, err := getCommitBytesWithoutSignature(commit)
		if err != nil {
			return errors.Join(ErrVerifyingSSHSignature, err)
		}
		commitSignature := []byte(commit.PGPSignature)

		if err := VerifySSHKeySignature(key, commitContents, commitSignature); err != nil {
			return errors.Join(ErrIncorrectVerificationKey, err)
		}

		return nil
	case signerverifier.FulcioKeyType:
		commitContents, err := getCommitBytesWithoutSignature(commit)
		if err != nil {
			return errors.Join(ErrVerifyingSigstoreSignature, err)
		}
		commitSignature := []byte(commit.PGPSignature)

		if err := VerifyGitsignSignature(ctx, key, commitContents, commitSignature); err != nil {
			return errors.Join(ErrIncorrectVerificationKey, err)
		}

		return nil
	}

	return ErrUnknownSigningMethod
}

func getCommitBytesWithoutSignature(commit *object.Commit) ([]byte, error) {
	commitEncoded := memory.NewStorage().NewEncodedObject()
	if err := commit.EncodeWithoutSignature(commitEncoded); err != nil {
		return nil, err
	}
	r, err := commitEncoded.Reader()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(r)
}
