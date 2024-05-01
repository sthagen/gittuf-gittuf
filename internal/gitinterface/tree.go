// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/gittuf/gittuf/internal/dev"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

// WriteTree creates a Git tree with the specified entries. It sorts the entries
// prior to creating the tree.
func WriteTree(repo *git.Repository, entries []object.TreeEntry) (plumbing.Hash, error) {
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	obj := repo.Storer.NewEncodedObject()
	tree := object.Tree{
		Entries: entries,
	}
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// GetTree returns the requested tree object.
func GetTree(repo *git.Repository, treeID plumbing.Hash) (*object.Tree, error) {
	return repo.TreeObject(treeID)
}

// EmptyTree returns the hash of an empty tree in a Git repository.
// Note: it is generated on the fly rather than stored as a constant to support
// SHA-256 repositories in future.
func EmptyTree() plumbing.Hash {
	obj := memory.NewStorage().NewEncodedObject()
	tree := object.Tree{}
	tree.Encode(obj) //nolint:errcheck

	return obj.Hash()
}

// GetAllFilesInTree returns all filepaths and the corresponding hash in the
// specified tree.
func GetAllFilesInTree(tree *object.Tree) (map[string]plumbing.Hash, error) {
	treeWalker := object.NewTreeWalker(tree, true, nil)
	defer treeWalker.Close()

	files := map[string]plumbing.Hash{}

	for {
		// This is based on FilesIter in go-git. That implementation loads each
		// blob which we don't want to do.
		name, entry, err := treeWalker.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		if entry.Mode == filemode.Dir || entry.Mode == filemode.Submodule {
			continue
		}

		files[name] = entry.Hash
	}

	return files, nil
}

func (r *Repository) GetAllFilesInTree(treeID string) (map[string]string, error) {
	stdOut, stdErr, err := r.executeGitCommand("ls-tree", "-r", "--format='%(path) %(objectname)'", treeID)
	if err != nil {
		return nil, fmt.Errorf("unable to enumerate all files in tree: %s", stdErr)
	}
	stdOut = strings.TrimSpace(stdOut)

	entries := strings.Split(stdOut, "\n")
	if len(entries) == 0 {
		return nil, nil
	}

	files := map[string]string{}
	for _, entry := range entries {
		entrySplit := strings.Split(entry, " ")
		// we control entry's format in --format above
		files[entrySplit[0]] = entrySplit[1]
	}

	return files, nil
}

// GetMergeTree computes the merge tree for the commits passed in. The tree is
// not written to the object store. Assuming a typical merge workflow, the first
// commit is expected to be the tip of the base branch. As such, the second
// commit is expected to be merged into the first. If the first commit is zero,
// the second commit's tree is returned.
func GetMergeTree(repo *git.Repository, commitAID, commitBID string) (string, error) {
	if !dev.InDevMode() {
		return "", dev.ErrNotInDevMode
	}

	// Base branch commit ID is zero
	if plumbing.NewHash(commitAID).IsZero() {
		// Return commitB's tree
		commitB, err := GetCommit(repo, plumbing.NewHash(commitBID))
		if err != nil {
			return "", err
		}

		return commitB.TreeHash.String(), nil
	}

	// go-git does not support three way merges
	command := exec.Command("git", "merge-tree", commitAID, commitBID) //nolint:gosec
	stdOut, err := command.Output()
	if err != nil {
		return "", err
	}

	stdOutString := strings.TrimSpace(string(stdOut))
	return stdOutString, nil
}

// TreeBuilder is used to create multi-level trees in a repository.
// Based on `buildTreeHelper` in go-git.
type TreeBuilder struct {
	r       *git.Repository
	trees   map[string]*object.Tree
	entries map[string]*object.TreeEntry
}

// NewTreeBuilder returns a TreeBuilder instance for the repository.
func NewTreeBuilder(repo *git.Repository) *TreeBuilder {
	return &TreeBuilder{r: repo}
}

// WriteRootTreeFromBlobIDs accepts a map of paths to their blob IDs and returns
// the root tree ID that contains these files.
func (t *TreeBuilder) WriteRootTreeFromBlobIDs(files map[string]plumbing.Hash) (plumbing.Hash, error) {
	rootNodeKey := ""
	t.trees = map[string]*object.Tree{rootNodeKey: {}}
	t.entries = map[string]*object.TreeEntry{}

	for path, blobID := range files {
		t.buildIntermediates(path, blobID)
	}

	return t.writeTrees(rootNodeKey, t.trees[rootNodeKey])
}

// buildIntermediates identifies the intermediate trees that must be constructed
// for the specified path.
func (t *TreeBuilder) buildIntermediates(name string, blobID plumbing.Hash) {
	parts := strings.Split(name, "/")

	var fullPath string
	for _, part := range parts {
		parent := fullPath
		fullPath = path.Join(fullPath, part)

		t.buildTree(name, parent, fullPath, blobID)
	}
}

// buildTree populates tree and entry information for each tree that must be
// created.
func (t *TreeBuilder) buildTree(name, parent, fullPath string, blobID plumbing.Hash) {
	if _, ok := t.trees[fullPath]; ok {
		return
	}

	if _, ok := t.entries[fullPath]; ok {
		return
	}

	entry := object.TreeEntry{Name: path.Base(fullPath)}

	if fullPath == name {
		entry.Mode = filemode.Regular
		entry.Hash = blobID
	} else {
		entry.Mode = filemode.Dir
		t.trees[fullPath] = &object.Tree{}
	}

	t.trees[parent].Entries = append(t.trees[parent].Entries, entry)
}

// writeTrees recursively stores each tree that must be created in the
// repository's object store. It returns the ID of the tree created at each
// invocation.
func (t *TreeBuilder) writeTrees(parent string, tree *object.Tree) (plumbing.Hash, error) {
	for i, e := range tree.Entries {
		if e.Mode != filemode.Dir && !e.Hash.IsZero() {
			continue
		}

		p := path.Join(parent, e.Name)
		entryID, err := t.writeTrees(p, t.trees[p])
		if err != nil {
			return plumbing.ZeroHash, err
		}
		e.Hash = entryID

		tree.Entries[i] = e
	}

	return WriteTree(t.r, tree.Entries)
}

type ReplacementTreeBuilder struct {
	repo    *Repository
	trees   map[string]*entry
	entries map[string]*entry
}

func NewReplacementTreeBuilder(repo *Repository) *ReplacementTreeBuilder {
	return &ReplacementTreeBuilder{repo: repo}
}

func (t *ReplacementTreeBuilder) WriteRootTreeFromBlobIDs(files map[string]string) (string, error) {
	rootNoteKey := ""
	t.trees = map[string]*entry{rootNoteKey: {}}
	t.entries = map[string]*entry{}

	for path, gitID := range files {
		t.buildIntermediates(path, gitID)
	}

	return t.writeTrees(rootNoteKey, t.trees[rootNoteKey])
}

func (t *ReplacementTreeBuilder) buildIntermediates(name, gitID string) {
	parts := strings.Split(name, "/")

	var fullPath string
	for _, part := range parts {
		parent := fullPath
		fullPath = path.Join(fullPath, part)

		t.buildTree(name, parent, fullPath, gitID)
	}
}

func (t *ReplacementTreeBuilder) buildTree(name, parent, fullPath, gitID string) {
	if _, ok := t.trees[fullPath]; ok {
		return
	}

	if _, ok := t.entries[fullPath]; ok {
		return
	}

	entryObj := &entry{name: path.Base(fullPath)}

	if fullPath == name {
		entryObj.isDir = false
		entryObj.gitID = gitID
	} else {
		entryObj.isDir = true
		t.trees[fullPath] = &entry{}
	}

	t.trees[parent].entries = append(t.trees[parent].entries, entryObj)
}

func (t *ReplacementTreeBuilder) writeTrees(parent string, tree *entry) (string, error) {
	for i, e := range tree.entries {
		if !e.isDir && e.gitID != "" {
			continue
		}

		p := path.Join(parent, e.name)
		entryID, err := t.writeTrees(p, t.trees[p])
		if err != nil {
			return "", err
		}
		e.gitID = entryID

		tree.entries[i] = e
	}

	return t.writeTree(tree.entries)
}

func (t *ReplacementTreeBuilder) writeTree(entries []*entry) (string, error) {
	input := ""
	for _, entry := range entries {
		if entry.isDir {
			input += "040000 tree " + entry.gitID + "    " + entry.name
		} else {
			input += "100644 blob " + entry.gitID + "    " + entry.name
		}
		input += "\n"
	}

	stdOut, stdErr, err := t.repo.executeGitCommandWithStdIn([]byte(input), "mk-tree")
	if err != nil {
		return "", fmt.Errorf("unable to write Git tree: %s", stdErr)
	}

	treeID := strings.TrimSpace(stdOut)
	return treeID, nil
}

type entry struct {
	name    string
	isDir   bool
	gitID   string
	entries []*entry // only used when isDir is true
}
