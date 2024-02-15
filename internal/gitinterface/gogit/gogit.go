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

func NewGoGitClient() (*GoGitClient, error) {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}

	return &GoGitClient{repository: repo}, nil
}

func NewGoGitClientForRepository(repo *git.Repository) *GoGitClient {
	return &GoGitClient{repository: repo}
}
