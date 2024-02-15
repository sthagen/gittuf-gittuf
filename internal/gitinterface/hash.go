// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"github.com/go-git/go-git/v5/plumbing"
)

type Hash interface {
	IsZero() bool
	String() string
}

type SHA1Hash = plumbing.Hash
