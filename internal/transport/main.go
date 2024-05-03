// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: %s <remote-name> <url>", os.Args[0])
	}

	var gitDir string

	gitDirEnv := os.Getenv("GIT_DIR")
	if gitDirEnv == "" {
		cmd := exec.Command("git", "rev-parse", "--git-dir")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("unable to identify GIT_DIR")
		}

		gitDir = strings.TrimSpace(string(output))
	} else {
		gitDir = gitDirEnv
	}

	remoteName := os.Args[1]
	url := os.Args[2]

	localTransportDir := filepath.Join(gitDir, "gittuf", remoteName)
	if err := os.MkdirAll(localTransportDir, 0o755); err != nil {
		return fmt.Errorf("unable to make transport directory: %w", err)
	}

	refSpec := fmt.Sprintf("refs/heads/*:refs/gittuf/transport/%s/*", remoteName)

	if err := os.Setenv("GIT_DIR", path.Join(url, ".git")); err != nil {
		return err
	}

	gitMarks := filepath.Join(localTransportDir, "git.marks")
	gittufTransportMarks := filepath.Join(localTransportDir, "gittuf.marks")

	if err := touch(gitMarks); err != nil {
		return err
	}
	if err := touch(gittufTransportMarks); err != nil {
		return err
	}

	// TODO: store and write original contents in case of error from here

	stdInReader := bufio.NewReader(os.Stdin)

	for {
		command, err := stdInReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("unable to read command from stdin: %w", err)
		}

		// where is the remote loop on the refs/gittuf/reference-state-log?
		// we need to know our entries are after upstream pushes
		switch {
		case command == "capabilities\n":
			fmt.Fprintf(os.Stdout, "import\n")
			fmt.Fprintf(os.Stdout, "export\n")
			fmt.Fprintf(os.Stdout, "refspec %s\n", refSpec)
			fmt.Fprintf(os.Stdout, "*import-marks %s\n", gitMarks)
			fmt.Fprintf(os.Stdout, "*export-marks %s\n", gitMarks)

			fmt.Fprintf(os.Stdout, "\n")
		case command == "list\n":
			refs, err := gitListRefs()
			if err != nil {
				return fmt.Errorf("error listing remote refs: %w", err)
			}

			head, err := gitSymbolicRef("HEAD")
			if err != nil {
				return fmt.Errorf("error resolving HEAD: %w", err)
			}

			for ref := range refs {
				fmt.Fprintf(os.Stdout, "? %s\n", ref)
			}
			fmt.Fprintf(os.Stdout, "@%s HEAD\n", head)

			fmt.Fprintf(os.Stdout, "\n")
		case strings.HasPrefix(command, "import "):
			// this is where we see refs being fetched? hook in verification?
			refs := []string{}
			for {
				ref := strings.TrimSpace(strings.TrimPrefix(command, "import "))
				refs = append(refs, ref)
				command, err = stdInReader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("unable to read command from stdin: %w", err)
				}

				if !strings.HasPrefix(command, "import ") {
					break
				}
			}

			// should refs always include `refs/gittuf/*` here?

			fmt.Fprintf(os.Stdout, "feature import-marks=%s\n", gitMarks)
			fmt.Fprintf(os.Stdout, "feature export-marks=%s\n", gitMarks)
			fmt.Fprintf(os.Stdout, "feature done\n")

			args := []string{"fast-export", "--import-marks", gittufTransportMarks, "--export-marks", gittufTransportMarks, "--refspec", refSpec}
			args = append(args, refs...)

			cmd := exec.Command("git", args...)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("unable to execute fast-export: %w", err)
			}

			fmt.Fprintf(os.Stdout, "done\n")
		case strings.HasPrefix(command, "export "):
			// AIUI, this is for exporting on push etc
			// we want to record an RSL entry if signed push isn't enabled
			// we want to verify what we're pushing
			beforeRefs, err := gitListRefs()
			if err != nil {
				return fmt.Errorf("unable to collect refs: %w", err)
			}

			cmd := exec.Command("git", "fast-import", "--quiet", "--import-marks="+gittufTransportMarks, "--export-marks="+gittufTransportMarks)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("unable to execute fast-import: %w", err)
			}

			afterRefs, err := gitListRefs()
			if err != nil {
				return fmt.Errorf("unable to collect refs: %w", err)
			}

			for refName, objectID := range afterRefs {
				// check assumptions about unknown refs here
				// also, evaluate where refs/gittuf/* gets plugged in
				if beforeRefs[refName] != objectID {
					fmt.Fprintf(os.Stdout, "ok %s\n", refName)
				}
			}

			fmt.Fprintf(os.Stdout, "\n")
		case command == "\n":
			return nil
		default:
			return fmt.Errorf("received unknown command '%s'", command)
		}
	}
}

func touch(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if os.IsExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	return file.Close()
}

func gitListRefs() (map[string]string, error) {
	output, err := exec.Command("git", "for-each-ref", "--format=%(objectname) %(refname)", "refs/heads/").Output()
	if err != nil {
		return nil, fmt.Errorf("unable to list refs: %w", err)
	}

	lines := bytes.Split(output, []byte{'\n'})
	refs := make(map[string]string, len(lines))

	for _, line := range lines {
		fields := bytes.Split(line, []byte{' '})
		if len(fields) < 2 {
			// trailing new line
			break
		}

		refs[string(fields[1])] = string(fields[0])
	}

	return refs, nil
}

func gitSymbolicRef(name string) (string, error) {
	output, err := exec.Command("git", "symbolic-ref", name).Output()
	if err != nil {
		return "", fmt.Errorf("unable to resolve symbolic ref: %w", err)
	}

	return string(bytes.TrimSpace(output)), nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
