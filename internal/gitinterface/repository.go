// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/jonboulle/clockwork"
)

const (
	binary           = "git"
	committerTimeKey = "GIT_COMMITTER_DATE"
	authorTimeKey    = "GIT_AUTHOR_DATE"
)

// Repository is a lightweight wrapper around a Git repository. It stores the
// location of the repository's GIT_DIR.
type Repository struct {
	gitDirPath string
	clock      clockwork.Clock
}

// GetGoGitRepository returns the go-git representation of a repository. We use
// this in certain signing and verifying workflows.
func (r *Repository) GetGoGitRepository() (*git.Repository, error) {
	return git.PlainOpenWithOptions(r.gitDirPath, &git.PlainOpenOptions{DetectDotGit: true})
}

// GetGitDir returns the GIT_DIR path for the repository.
func (r *Repository) GetGitDir() string {
	return r.gitDirPath
}

// LoadRepository returns a Repository instance using the current working
// directory. It also inspects the PATH to ensure Git is installed.
func LoadRepository() (*Repository, error) {
	_, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("unable to find Git binary, is Git installed?")
	}

	repo := &Repository{clock: clockwork.NewRealClock()}
	envVar := os.Getenv("GIT_DIR")
	if envVar != "" {
		repo.gitDirPath = envVar
		return repo, nil
	}

	stdOut, stdErr, err := repo.executeGitCommandDirect("rev-parse", "--git-dir")
	if err != nil {
		return nil, fmt.Errorf("unable to identify GIT_DIR: %w: %s", err, stdErr)
	}
	repo.gitDirPath = strings.TrimSpace(stdOut)

	return repo, nil
}

// executeGitCommand is a helper to execute the specified command in the
// repository. It automatically adds the explicit `--git-dir` parameter.
func (r *Repository) executeGitCommand(args ...string) (string, string, error) {
	args = append([]string{"--git-dir", r.gitDirPath}, args...)
	return r.executeGitCommandDirect(args...)
}

// executeGitCommandDirect is a helper to execute the specified command in the
// repository. It executes in the current directory without specifying the
// GIT_DIR explicitly.
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

// executeGitCommandWithStdIn is a helper to execute the specified command in
// the repository with `stdInContents` passed into the process stdin. It
// automatically adds the explicit `--git-dir` parameter.
func (r *Repository) executeGitCommandWithStdIn(stdInContents []byte, args ...string) (string, string, error) {
	args = append([]string{"--git-dir", r.gitDirPath}, args...)
	return r.executeGitCommandDirectWithStdIn(stdInContents, args...)
}

// executeGitCommandDirectWithStdIn is a helper to execute the specified command
// in the repository with `stdInContents` passed into the process stdin. It
// executes in the current directory without specifying the GIT_DIR explicitly.
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
