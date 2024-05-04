// SPDX-License-Identifier: Apache-2.0

// Learning about git remote helpers
// This is based off https://rovaughn.github.io/2015-2-9.html, which seems to be
// the most definitive docs for something like this.
// Annotating where I think gittuf would plug in

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var logFile io.Writer

func run() (reterr error) {
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

	refSpecs := []string{
		fmt.Sprintf("refs/heads/*:refs/gittuf-transport/%s/heads/*", remoteName),
		"refs/gittuf/*:refs/gittuf/*",
	}

	// how does this work for remotes over the network?
	// if err := os.Setenv("GIT_DIR", path.Join(url, ".git")); err != nil {
	// 	return err
	// }

	// gitMarks := filepath.Join(localTransportDir, "git.marks")
	// gittufTransportMarks := filepath.Join(localTransportDir, "gittuf.marks")

	// if err := touch(gitMarks); err != nil {
	// 	return err
	// }
	// if err := touch(gittufTransportMarks); err != nil {
	// 	return err
	// }

	// originalGitMarks, err := os.ReadFile(gitMarks)
	// if err != nil {
	// 	return err
	// }
	// originalGittufTransportMarks, err := os.ReadFile(gittufTransportMarks)
	// if err != nil {
	// 	return err
	// }

	// defer func() {
	// 	log("writing original marks files")
	// 	if reterr != nil {
	// 		os.WriteFile(gitMarks, originalGitMarks, 0o666)
	// 		os.WriteFile(gittufTransportMarks, originalGittufTransportMarks, 0o666)
	// 	}
	// }()

	stdInReader := bufio.NewReader(os.Stdin)

	log("entering loop")
	for {
		command, err := stdInReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("unable to read command from stdin: %w", err)
		}

		log("command: " + strings.TrimSpace(command))

		// where is the remote loop on the refs/gittuf/reference-state-log?
		// we need to know our entries are after upstream pushes
		switch {
		case command == "capabilities\n":
			logAndWrite("fetch\n")
			logAndWrite("push\n")
			// logAndWrite("import\n")
			// logAndWrite("export\n")
			for _, refSpec := range refSpecs {
				logAndWrite(fmt.Sprintf("refspec %s\n", refSpec))
			}
			// logAndWrite(fmt.Sprintf("*import-marks %s\n", gitMarks))
			// logAndWrite(fmt.Sprintf("*export-marks %s\n", gitMarks))

			fmt.Fprintf(os.Stdout, "\n")

		case command == "list\n", command == "list for-push\n":
			refs, err := gitListRefs()
			if err != nil {
				return fmt.Errorf("error listing remote refs: %w", err)
			}

			head, err := gitSymbolicRef("HEAD")
			if err != nil {
				return fmt.Errorf("error resolving HEAD: %w", err)
			}

			for ref := range refs {
				logAndWrite(fmt.Sprintf("? %s\n", ref))
			}
			logAndWrite(fmt.Sprintf("@%s HEAD\n", head))

			fmt.Fprintf(os.Stdout, "\n")

		case strings.HasPrefix(command, "fetch "):
			refs := []string{"refs/gittuf/reference-state-log", "refs/gittuf/policy", "refs/gittuf/policy-staging", "refs/gittuf/attestations"}
			requestedRefs := []string{}

			for {
				fetchRequest := strings.TrimSpace(strings.TrimPrefix(command, "fetch "))

				parts := strings.Split(fetchRequest, " ")
				if len(parts) < 2 {
					return fmt.Errorf("malformed fetch request: %s", fetchRequest)
				}

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
			args := []string{"fetch-pack", url}
			args = append(args, refs...)
			args = append(args, requestedRefs...)
			log(strings.Join(args, " "))
			cmd := exec.Command("git", args...)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("unable to execute fetch-pack: %w", err)
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

		// case strings.HasPrefix(command, "import "):
		// 	// This is old, will remove
		// 	// import recreates commits locally, breaks sigs

		// 	refs := []string{"refs/gittuf/reference-state-log", "refs/gittuf/policy", "refs/gittuf/policy-staging", "refs/gittuf/attestations"}

		// 	requestedRefs := []string{}

		// 	for {
		// 		ref := strings.TrimSpace(strings.TrimPrefix(command, "import "))
		// 		refs = append(refs, ref)
		// 		requestedRefs = append(requestedRefs, ref)

		// 		command, err = stdInReader.ReadString('\n')
		// 		if err != nil {
		// 			return fmt.Errorf("unable to read command from stdin: %w", err)
		// 		}

		// 		log("import command: " + strings.TrimSpace(command))

		// 		if command == "\n" {
		// 			break
		// 		}

		// 		if !strings.HasPrefix(command, "import ") {
		// 			return fmt.Errorf("received non import command in import batch: '%s'", command)
		// 		}
		// 	}

		// 	// should refs always include `refs/gittuf/*` here?

		// 	logAndWrite(fmt.Sprintf("feature import-marks=%s\n", gitMarks))
		// 	logAndWrite(fmt.Sprintf("feature export-marks=%s\n", gitMarks))
		// 	logAndWrite("feature done\n")

		// 	args := []string{"fast-export", "--import-marks", gittufTransportMarks, "--export-marks", gittufTransportMarks}
		// 	for _, refSpec := range refSpecs {
		// 		args = append(args, "--refspec", refSpec)
		// 	}
		// 	args = append(args, refs...)
		// 	log(strings.Join(args, " "))

		// 	cmd := exec.Command("git", args...)
		// 	cmd.Stderr = os.Stderr
		// 	cmd.Stdout = os.Stdout

		// 	if err := cmd.Run(); err != nil {
		// 		return fmt.Errorf("unable to execute fast-export: %w", err)
		// 	}

		// 	logAndWrite("done\n")

		// 	for _, ref := range requestedRefs {
		// 		if err := exec.Command("gittuf", "verify-ref", ref).Run(); err != nil {
		// 			return fmt.Errorf("error verifying gittuf policies: %w", err)
		// 		}
		// 	}

		// case command == "export\n":
		// 	// This is old, will remove
		// 	// export recreates commits on remote, breaks sigs

		// 	// AIUI, this is for exporting on push etc
		// 	// we want to record an RSL entry if signed push isn't enabled
		// 	// we want to verify what we're pushing
		// 	beforeRefs, err := gitListRefs()
		// 	if err != nil {
		// 		return fmt.Errorf("unable to collect refs: %w", err)
		// 	}

		// 	s := []string{}
		// 	for key, val := range beforeRefs {
		// 		s = append(s, key+" "+val)
		// 	}
		// 	log("beforeRefs: " + strings.Join(s, ", "))

		// 	args := []string{"fast-import", "--quiet", "--import-marks=" + gittufTransportMarks, "--export-marks=" + gittufTransportMarks}
		// 	log(strings.Join(args, " "))
		// 	cmd := exec.Command("git", args...)
		// 	cmd.Stderr = os.Stderr
		// 	cmd.Stdout = os.Stdout

		// 	if err := cmd.Run(); err != nil {
		// 		return fmt.Errorf("unable to execute fast-import: %w", err)
		// 	}

		// 	afterRefs, err := gitListRefs()
		// 	if err != nil {
		// 		return fmt.Errorf("unable to collect refs: %w", err)
		// 	}

		// 	s = []string{}
		// 	for key, val := range beforeRefs {
		// 		s = append(s, key+" "+val)
		// 	}
		// 	log("afterRefs: " + strings.Join(s, ", "))

		// 	for refName, objectID := range afterRefs {
		// 		// check assumptions about unknown refs here
		// 		// also, evaluate where refs/gittuf/* gets plugged in
		// 		if beforeRefs[refName] != objectID {
		// 			logAndWrite(fmt.Sprintf("ok %s\n", refName))
		// 		}
		// 	}

		// 	fmt.Fprintf(os.Stdout, "\n")

		case command == "\n":
			return nil

		default:
			return fmt.Errorf("received unknown command '%s'", strings.TrimSpace(command))
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
	log("gitDir: " + os.Getenv("GIT_DIR"))
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

func logAndWrite(message string) {
	log(strings.TrimSpace(message))
	fmt.Fprint(os.Stdout, message)
}

func log(message string) {
	fmt.Fprint(logFile, message+"\n")
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
