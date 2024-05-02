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

func (r *Repository) SetReference(refName, gitID string) error {
	_, stdErr, err := r.executeGitCommand("update-ref", "--create-reflog", refName, gitID)
	if err != nil {
		return fmt.Errorf("unable to set Git reference '%s' to '%s': %s", refName, gitID, stdErr)
	}

	return nil
}

func (r *Repository) CheckAndSetReference(refName, newGitID, oldGitID string) error {
	_, stdErr, err := r.executeGitCommand("update-ref", "--create-reflog", refName, newGitID, oldGitID)
	if err != nil {
		return fmt.Errorf("unable to set Git reference '%s' to '%s': %s", refName, newGitID, stdErr)
	}

	return nil
}

func (r *Repository) GetReference(refName string) (string, error) {
	stdOut, stdErr, err := r.executeGitCommand("rev-parse", refName)
	if err != nil {
		if strings.Contains(stdErr, "unknown revision or path not in the working tree") {
			return "", ErrReferenceNotFound
		}
		return "", fmt.Errorf("unable to read reference '%s': %s", refName, stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}
