// SPDX-License-Identifier: Apache-2.0

// Learning about git remote helpers
// This is based off https://rovaughn.github.io/2015-2-9.html, which seems to be
// the most definitive docs for something like this.
// Annotating where I think gittuf would plug in

// Sources:
// https://rovaughn.github.io/2015-2-9.html
// https://github.com/keybase/client/blob/master/go/kbfs/kbfsgit/runner.go
// https://github.com/spwhitton/git-remote-gcrypt/blob/master/git-remote-gcrypt

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
)

var logFile io.Writer

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: %s <remote-name> <url>", os.Args[0])
	}

	url := os.Args[2]

	refSpecs := []string{
		"refs/heads/*:refs/heads/*",
		"refs/gittuf/*:refs/gittuf/*",
	}

	stdInReader := bufio.NewReader(os.Stdin)

	log("entering helper loop")
	for {
		command, err := stdInReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("unable to read command from stdin: %w", err)
		}

		if command != "\n" {
			log("command: " + strings.TrimSpace(command))
		}

		switch {
		case command == "capabilities\n":
			logAndWrite("fetch\n")
			logAndWrite("push\n")
			for _, refSpec := range refSpecs {
				logAndWrite(fmt.Sprintf("refspec %s\n", refSpec))
			}

			fmt.Fprintf(os.Stdout, "\n")

		case command == "list\n", command == "list for-push\n":
			// this is likely problematic, I'm not sure i fully understand where
			// this is expected to be run
			// when `list`-ing for `fetch`, is this listing the remote's refs?
			// we need to solve the "actual" transport to make sense of this
			// also, all of this is naturally only for a "smart" protocol?

			refs, err := gitListRefs(url)
			if err != nil {
				return fmt.Errorf("error listing remote refs: %w", err)
			}

			for ref := range refs {
				logAndWrite(fmt.Sprintf("? %s\n", ref))
			}

			head, err := gitSymbolicRef("HEAD", url)
			if err == nil {
				logAndWrite(fmt.Sprintf("@%s HEAD\n", head))
			}

			fmt.Fprintf(os.Stdout, "\n")

		case strings.HasPrefix(command, "fetch "):
			gittufRefs := []string{
				"refs/gittuf/reference-state-log",
				"refs/gittuf/policy",
				"refs/gittuf/policy-staging",
				"refs/gittuf/attestations",
			}
			requestedRefs := []string{}

			// this may fetch too many refs, not just the default as it lists remote-refs

			for {
				fetchRequest := strings.TrimSpace(strings.TrimPrefix(command, "fetch "))

				parts := strings.Split(fetchRequest, " ")
				if len(parts) < 2 {
					return fmt.Errorf("malformed fetch request: %s", fetchRequest)
				}

				log("fetch request: " + fetchRequest)
				requestedRefs = append(requestedRefs, parts[1])

				command, err = stdInReader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("unable to read command from stdin: %w", err)
				}

				log("fetch command: " + strings.TrimSpace(command))

				if command == "\n" {
					break
				}

				if !strings.HasPrefix(command, "fetch ") {
					return fmt.Errorf("received non fetch command in fetch batch: '%s'", command)
				}
			}

			log("invoking fetch-pack")
			// fetch pack looks at refs rather than src:dst refspec
			// it's populating the object store, so this makes sense
			// we have to update local refs ourselves with update-ref after?
			args := []string{"fetch-pack", url}
			args = append(args, gittufRefs...)
			args = append(args, requestedRefs...)
			log(strings.Join(args, " "))
			cmd := exec.Command("git", args...)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("unable to execute fetch-pack: %w", err)
			}

			// don't we need to be able to list / for-each-ref on the remote to
			// learn what to set locals to?
			targetRefs, err := gitListRefs(url)
			if err != nil {
				return fmt.Errorf("unable to list remote refs: %w", err)
			}

			for _, ref := range append(gittufRefs, requestedRefs...) {
				targetObj, listed := targetRefs[ref]
				if !listed {
					// remote doesn't have this ref??
					continue
				}

				args := []string{"update-ref", ref, targetObj} // should also include oldOid for checkandsetref...
				cmd := exec.Command("git", args...)
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout

				if err := cmd.Run(); err != nil {
					return fmt.Errorf("unable to update local ref '%s': %w", ref, err)
				}
			}

			fmt.Fprintf(os.Stdout, "\n")

		case strings.HasPrefix(command, "push "):
			refSpecs := []string{
				// could just refs/gittuf/* but we also need to implement the
				// fetch RSL loop to ensure entry is the latest one
				"refs/gittuf/reference-state-log:refs/gittuf/reference-state-log",
				"refs/gittuf/policy:refs/gittuf/policy",
				"refs/gittuf/policy-staging:refs/gittuf/policy-staging",
				"refs/gittuf/attestations:refs/gittuf/attestations",
			}

			requestedPushRefSpecs := []string{}

			for {
				pushRequest := strings.TrimSpace(strings.TrimPrefix(command, "push "))
				requestedPushRefSpecs = append(requestedPushRefSpecs, pushRequest)

				command, err = stdInReader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("unable to read command from stdin: %w", err)
				}

				log("push command: " + strings.TrimSpace(command))

				if command == "\n" {
					break
				}

				if !strings.HasPrefix(command, "push ") {
					return fmt.Errorf("received non push command in push batch: '%s'", command)
				}
			}

			// Check remote RSL, create local RSL entry

			args := []string{"send-pack", "--atomic", url}
			args = append(args, refSpecs...)
			args = append(args, requestedPushRefSpecs...)
			log(strings.Join(args, " "))
			cmd := exec.Command("git", args...)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("unable to execute send-pack: %w", err)
			}

			fmt.Fprintf(os.Stdout, "\n")

		case command == "\n":
			return nil

		default:
			return fmt.Errorf("received unknown command '%s'", strings.TrimSpace(command))
		}
	}
}

func gitListRefs(repoLocation string) (map[string]string, error) {
	_, err := os.Stat(path.Join(repoLocation, ".git"))
	if err == nil {
		repoLocation = path.Join(repoLocation, ".git")
	}
	cmd := exec.Command("git", "--git-dir", repoLocation, "for-each-ref", "--format=%(objectname) %(refname)", "refs/heads/")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("unable to list refs: %s", string(err.(*exec.ExitError).Stderr))
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

func gitSymbolicRef(name, repoLocation string) (string, error) {
	_, err := os.Stat(path.Join(repoLocation, ".git"))
	if err == nil {
		repoLocation = path.Join(repoLocation, ".git")
	}
	cmd := exec.Command("git", "--git-dir", repoLocation, "symbolic-ref", name)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("unable to resolve symbolic ref: %s", string(err.(*exec.ExitError).Stderr))
	}

	return string(bytes.TrimSpace(output)), nil
}

func logAndWrite(message string) {
	log(strings.TrimSpace(message))
	fmt.Fprint(os.Stdout, message)
}

func log(message string) {
	if logFile != nil {
		fmt.Fprint(logFile, message+"\n")
	}
}

func main() {
	logFilePath := os.Getenv("GITTUF_LOG_FILE")
	if logFilePath != "" {
		file, err := os.Create(logFilePath)
		if err != nil {
			panic(err)
		}

		logFile = file
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
