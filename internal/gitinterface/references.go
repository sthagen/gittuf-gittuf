// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrReferenceNotFound = errors.New("requested Git reference not found")
)

func (r *Repository) SetReference(refName string, gitID Hash) error {
	_, stdErr, err := r.executeGitCommand("update-ref", "--create-reflog", refName, gitID.String())
	if err != nil {
		return fmt.Errorf("unable to set Git reference '%s' to '%s': %s", refName, gitID.String(), stdErr)
	}

	return nil
}

func (r *Repository) CheckAndSetReference(refName string, newGitID, oldGitID Hash) error {
	_, stdErr, err := r.executeGitCommand("update-ref", "--create-reflog", refName, newGitID.String(), oldGitID.String())
	if err != nil {
		return fmt.Errorf("unable to set Git reference '%s' to '%s': %s", refName, newGitID.String(), stdErr)
	}

	return nil
}

func (r *Repository) GetReference(refName string) (Hash, error) {
	stdOut, stdErr, err := r.executeGitCommand("rev-parse", refName)
	if err != nil {
		if strings.Contains(stdErr, "unknown revision or path not in the working tree") {
			return ZeroHash, ErrReferenceNotFound
		}
		return ZeroHash, fmt.Errorf("unable to read reference '%s': %s", refName, stdErr)
	}

	hash, err := NewHash(strings.TrimSpace(stdOut))
	if err != nil {
		return ZeroHash, fmt.Errorf("invalid Git ID for reference '%s': %w", refName, err)
	}

	return hash, nil
}
