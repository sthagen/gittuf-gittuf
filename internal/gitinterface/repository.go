// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	ZeroHash = "0000000000000000000000000000000000000000"

	binary = "git"
)

type Repository struct {
	gitDirPath string
}

func LoadRepository() (*Repository, error) {
	envVar := os.Getenv("GIT_DIR")
	if envVar != "" {
		return &Repository{gitDirPath: envVar}, nil
	}

	repo := &Repository{}

	stdOut, stdErr, err := repo.executeGitCommandDirect("rev-parse", "--git-dir")
	if err != nil {
		return nil, fmt.Errorf("unable to identify GIT_DIR: %w: %s", err, stdErr)
	}
	repo.gitDirPath = strings.TrimSpace(stdOut)

	return repo, nil
}

func (r *Repository) executeGitCommand(args ...string) (string, string, error) {
	args = append([]string{"--git-dir", r.gitDirPath}, args...)
	return r.executeGitCommandDirect(args...)
}

func (r *Repository) executeGitCommandDirect(args ...string) (string, string, error) {
	cmd := exec.Command(binary, args...)

	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)

	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	err := cmd.Run()
	stdOutString := stdOut.String() // sometimes we want the trailing new line (say when `cat-file -p` a blob, leaving it to the caller)
	stdErrString := strings.TrimSpace(stdErr.String())
	if err != nil {
		if stdErrString == "" {
			stdErrString = "error running `git " + strings.Join(args, " ") + "`"
		}
	}
	return stdOutString, stdErrString, err
}

func (r *Repository) executeGitCommandWithStdIn(stdInContents []byte, args ...string) (string, string, error) {
	args = append([]string{"--git-dir", r.gitDirPath}, args...)
	return r.executeGitCommandDirectWithStdIn(stdInContents, args...)
}

func (r *Repository) executeGitCommandDirectWithStdIn(stdInContents []byte, args ...string) (string, string, error) {
	cmd := exec.Command(binary, args...)

	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)

	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	stdInWriter, err := cmd.StdinPipe()
	if err != nil {
		return "", "", fmt.Errorf("unable to create stdin writer: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("error starting command: %w", err)
	}

	if _, err = stdInWriter.Write(stdInContents); err != nil {
		return "", "", fmt.Errorf("unable writing stdin contents: %w", err)
	}
	if err := stdInWriter.Close(); err != nil {
		return "", "", fmt.Errorf("unable to close stdin writer: %w", err)
	}

	err = cmd.Wait()
	stdOutString := stdOut.String() // sometimes we want the trailing new line (say when `cat-file -p` a blob, leaving it to the caller)
	stdErrString := strings.TrimSpace(stdErr.String())
	if err != nil {
		if stdErrString == "" {
			stdErrString = "error running `git " + strings.Join(args, " ") + "`"
		}
	}
	return stdOutString, stdErrString, err
}
