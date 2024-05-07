// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"encoding/hex"
	"errors"
)

const (
	zeroSHA1HashString   = "0000000000000000000000000000000000000000"
	zeroSHA256HashString = "0000000000000000000000000000000000000000000000000000000000000000"
)

var (
	ErrInvalidHashEncoding = errors.New("hash string is not hex encoded")
	ErrInvalidHashLength   = errors.New("hash string is wrong length")
)

// Hash represents a Git object hash. It is a lightweight wrapper around the
// standard hex encoded representation of a SHA-1 or SHA-256 hash used by Git.
type Hash struct {
	hash string
}

// String returns the hex encoded hash.
func (h Hash) String() string {
	return h.hash
}

// IsZero compares the hash to see if it's the zero hash for either SHA-1 or
// SHA-256.
func (h Hash) IsZero() bool {
	return h.hash == zeroSHA1HashString || h.hash == zeroSHA256HashString
}

// ZeroHash represents an empty Hash.
// TODO: use SHA-256 zero hash for repositories that have that as the default.
var ZeroHash = Hash{hash: zeroSHA1HashString}

// NewHash returns a Hash object after ensuring the input string is correctly
// encoded.
func NewHash(h string) (Hash, error) {
	if len(h) != len(zeroSHA1HashString) && len(h) != len(zeroSHA256HashString) {
		return ZeroHash, ErrInvalidHashLength
	}

	_, err := hex.DecodeString(h)
	if err != nil {
		return ZeroHash, ErrInvalidHashEncoding
	}

	return Hash{hash: h}, nil
}
