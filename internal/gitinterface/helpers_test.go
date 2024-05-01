// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"os"
	"os/exec"
	"path"
	"testing"
)

func createTestGitRepository(t *testing.T, dir string) *Repository {
	t.Helper()

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binary, "init")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	return &Repository{gitDirPath: path.Join(dir, ".git")}
}
