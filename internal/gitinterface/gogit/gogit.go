// SPDX-License-Identifier: Apache-2.0

package gogit

import (
	"github.com/go-git/go-git/v5"
	"github.com/jonboulle/clockwork"
)

var clock = clockwork.NewRealClock()

type GoGitClient struct {
	repository *git.Repository
}
