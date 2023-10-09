package rsl

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/gittuf/gittuf/internal/gitinterface"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	GittufNamespacePrefix      = "refs/gittuf/"
	Ref                        = "refs/gittuf/reference-state-log"
	EntryHeader                = "RSL Entry"
	RefKey                     = "ref"
	TargetIDKey                = "targetID"
	AnnotationHeader           = "RSL Annotation"
	AnnotationMessageBlockType = "MESSAGE"
	BeginMessage               = "-----BEGIN MESSAGE-----"
	EndMessage                 = "-----END MESSAGE-----"
	EntryIDKey                 = "entryID"
	SkipKey                    = "skip"

	remoteTrackerRef = "refs/remotes/%s/gittuf/reference-state-log"
)

var (
	ErrRSLExists               = errors.New("cannot initialize RSL namespace as it exists already")
	ErrRSLEntryNotFound        = errors.New("unable to find RSL entry")
	ErrRSLBranchDetected       = errors.New("potential RSL branch detected, entry has more than one parent")
	ErrInvalidRSLEntry         = errors.New("RSL entry has invalid format or is of unexpected type")
	ErrRSLEntryDoesNotMatchRef = errors.New("RSL entry does not match requested ref")
	ErrNoRecordOfCommit        = errors.New("commit has not been encountered before")
)

// InitializeNamespace creates a git ref for the reference state log. Initially,
// the entry has a zero hash.
func InitializeNamespace(repo *git.Repository) error {
	if _, err := repo.Reference(plumbing.ReferenceName(Ref), true); err != nil {
		if !errors.Is(err, plumbing.ErrReferenceNotFound) {
			return err
		}
	} else {
		return ErrRSLExists
	}

	return repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(Ref), plumbing.ZeroHash))
}

// RemoteTrackerRef returns the remote tracking ref for the specified remote
// name. For example, for 'origin', the remote tracker ref is
// 'refs/remotes/origin/gittuf/reference-state-log'.
func RemoteTrackerRef(remote string) string {
	return fmt.Sprintf(remoteTrackerRef, remote)
}

type EntryType interface {
	GetID() plumbing.Hash
	Commit(*git.Repository, bool) error
	createCommitMessage() (string, error)
}

type Entry struct {
	// ID contains the Git hash for the commit corresponding to the entry.
	ID plumbing.Hash

	// RefName contains the Git reference the entry is for.
	RefName string

	// TargetID contains the Git hash for the object expected at RefName.
	TargetID plumbing.Hash
}

// NewEntry returns an Entry object for a normal RSL entry.
func NewEntry(refName string, targetID plumbing.Hash) *Entry {
	return &Entry{RefName: refName, TargetID: targetID}
}

func (e *Entry) GetID() plumbing.Hash {
	return e.ID
}

// Commit creates a commit object in the RSL for the Entry.
func (e *Entry) Commit(repo *git.Repository, sign bool) error {
	message, _ := e.createCommitMessage() // we have an error return for annotations, always nil here

	_, err := gitinterface.Commit(repo, gitinterface.EmptyTree(), Ref, message, sign)
	return err
}

func (e *Entry) createCommitMessage() (string, error) {
	lines := []string{
		EntryHeader,
		"",
		fmt.Sprintf("%s: %s", RefKey, e.RefName),
		fmt.Sprintf("%s: %s", TargetIDKey, e.TargetID.String()),
	}
	return strings.Join(lines, "\n"), nil
}

type Annotation struct {
	// ID contains the Git hash for the commit corresponding to the annotation.
	ID plumbing.Hash

	// RSLEntryIDs contains one or more Git hashes for the RSL entries the annotation applies to.
	RSLEntryIDs []plumbing.Hash

	// Skip indicates if the RSLEntryIDs must be skipped during gittuf workflows.
	Skip bool

	// Message contains any messages or notes added by a user for the annotation.
	Message string
}

// NewAnnotation returns an Annotation object that applies to one or more prior
// RSL entries.
func NewAnnotation(rslEntryIDs []plumbing.Hash, skip bool, message string) *Annotation {
	return &Annotation{RSLEntryIDs: rslEntryIDs, Skip: skip, Message: message}
}

func (a *Annotation) GetID() plumbing.Hash {
	return a.ID
}

// Commit creates a commit object in the RSL for the Annotation.
func (a *Annotation) Commit(repo *git.Repository, sign bool) error {
	// Check if referred entries exist in the RSL namespace.
	for _, id := range a.RSLEntryIDs {
		if _, err := GetEntry(repo, id); err != nil {
			return err
		}
	}

	message, err := a.createCommitMessage()
	if err != nil {
		return err
	}

	_, err = gitinterface.Commit(repo, gitinterface.EmptyTree(), Ref, message, sign)
	return err
}

// RefersTo returns true if the specified entryID is referred to by the
// annotation.
func (a *Annotation) RefersTo(entryID plumbing.Hash) bool {
	for _, id := range a.RSLEntryIDs {
		if id == entryID {
			return true
		}
	}

	return false
}

func (a *Annotation) createCommitMessage() (string, error) {
	lines := []string{
		AnnotationHeader,
		"",
	}

	for _, entry := range a.RSLEntryIDs {
		lines = append(lines, fmt.Sprintf("%s: %s", EntryIDKey, entry.String()))
	}

	if a.Skip {
		lines = append(lines, fmt.Sprintf("%s: true", SkipKey))
	} else {
		lines = append(lines, fmt.Sprintf("%s: false", SkipKey))
	}

	if len(a.Message) != 0 {
		var message strings.Builder
		messageBlock := pem.Block{
			Type:  AnnotationMessageBlockType,
			Bytes: []byte(a.Message),
		}
		if err := pem.Encode(&message, &messageBlock); err != nil {
			return "", err
		}
		lines = append(lines, strings.TrimSpace(message.String()))
	}

	return strings.Join(lines, "\n"), nil
}

// GetEntry returns the entry corresponding to entryID.
func GetEntry(repo *git.Repository, entryID plumbing.Hash) (EntryType, error) {
	commitObj, err := repo.CommitObject(entryID)
	if err != nil {
		return nil, ErrRSLEntryNotFound
	}

	return parseRSLEntryText(entryID, commitObj.Message)
}

// GetParentForEntry returns the entry's parent RSL entry.
func GetParentForEntry(repo *git.Repository, entry EntryType) (EntryType, error) {
	commitObj, err := repo.CommitObject(entry.GetID())
	if err != nil {
		return nil, err
	}

	if len(commitObj.ParentHashes) == 0 {
		return nil, ErrRSLEntryNotFound
	}

	if len(commitObj.ParentHashes) > 1 {
		return nil, ErrRSLBranchDetected
	}

	return GetEntry(repo, commitObj.ParentHashes[0])
}

// GetNonGittufParentForEntry returns the first RSL entry starting from the
// specified entry's parent that is not for the gittuf namespace.
func GetNonGittufParentForEntry(repo *git.Repository, entry EntryType) (*Entry, []*Annotation, error) {
	it, err := GetParentForEntry(repo, entry)
	if err != nil {
		return nil, nil, err
	}

	allAnnotations := []*Annotation{}
	var targetEntry *Entry

	for {
		switch iterator := it.(type) {
		case *Entry:
			if !strings.HasPrefix(iterator.RefName, GittufNamespacePrefix) {
				targetEntry = iterator
			}
		case *Annotation:
			allAnnotations = append(allAnnotations, iterator)
		}

		if targetEntry != nil {
			// we've found the target entry, stop walking the RSL
			break
		}

		it, err = GetParentForEntry(repo, it)
		if err != nil {
			return nil, nil, err
		}
	}

	annotations := filterAnnotationsForRelevantAnnotations(allAnnotations, targetEntry.ID)

	return targetEntry, annotations, nil
}

// GetLatestEntry returns the latest entry available locally in the RSL.
func GetLatestEntry(repo *git.Repository) (EntryType, error) {
	ref, err := repo.Reference(plumbing.ReferenceName(Ref), true)
	if err != nil {
		return nil, err
	}

	commitObj, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, ErrRSLEntryNotFound
	}

	return parseRSLEntryText(commitObj.Hash, commitObj.Message)
}

// GetLatestNonGittufEntry returns the first RSL entry that is not for the
// gittuf namespace.
func GetLatestNonGittufEntry(repo *git.Repository) (*Entry, []*Annotation, error) {
	it, err := GetLatestEntry(repo)
	if err != nil {
		return nil, nil, err
	}

	allAnnotations := []*Annotation{}
	var targetEntry *Entry

	for {
		switch iterator := it.(type) {
		case *Entry:
			if !strings.HasPrefix(iterator.RefName, GittufNamespacePrefix) {
				targetEntry = iterator
			}
		case *Annotation:
			allAnnotations = append(allAnnotations, iterator)
		}

		if targetEntry != nil {
			// we've found the target entry, stop walking the RSL
			break
		}

		it, err = GetParentForEntry(repo, it)
		if err != nil {
			return nil, nil, err
		}
	}

	annotations := filterAnnotationsForRelevantAnnotations(allAnnotations, targetEntry.ID)

	return targetEntry, annotations, nil
}

// GetLatestEntryForRef returns the latest entry available locally in the RSL
// for the specified refName.
func GetLatestEntryForRef(repo *git.Repository, refName string) (*Entry, []*Annotation, error) {
	return GetLatestEntryForRefBefore(repo, refName, plumbing.ZeroHash)
}

// GetLatestEntryForRefBefore returns the latest entry available locally in the
// RSL for the specified refName before the specified anchor.
func GetLatestEntryForRefBefore(repo *git.Repository, refName string, anchor plumbing.Hash) (*Entry, []*Annotation, error) {
	var (
		iteratorT EntryType
		err       error
	)

	if anchor.IsZero() {
		iteratorT, err = GetLatestEntry(repo)
		if err != nil {
			return nil, nil, err
		}
	} else {
		iteratorT, err = GetEntry(repo, anchor)
		if err != nil {
			return nil, nil, err
		}

		// We have to set the iterator to the parent. The other option is to
		// swap the refName check and parent in the loop below but that breaks
		// GetLatestEntryForRef's behavior. By adding this one extra GetParent
		// here, we avoid repetition.
		iteratorT, err = GetParentForEntry(repo, iteratorT)
		if err != nil {
			return nil, nil, err
		}
	}

	allAnnotations := []*Annotation{}
	var targetEntry *Entry

	for {
		switch iterator := iteratorT.(type) {
		case *Entry:
			if iterator.RefName == refName {
				targetEntry = iterator
			}
		case *Annotation:
			allAnnotations = append(allAnnotations, iterator)
		}

		if targetEntry != nil {
			// we've found the target entry, stop walking the RSL
			break
		}

		iteratorT, err = GetParentForEntry(repo, iteratorT)
		if err != nil {
			return nil, nil, err
		}

	}

	annotations := filterAnnotationsForRelevantAnnotations(allAnnotations, targetEntry.ID)

	return targetEntry, annotations, nil
}

// GetFirstEntry returns the very first entry in the RSL. It is expected to be
// *Entry as the first entry in the RSL cannot be an annotation.
func GetFirstEntry(repo *git.Repository) (*Entry, []*Annotation, error) {
	iteratorT, err := GetLatestEntry(repo)
	if err != nil {
		return nil, nil, err
	}

	allAnnotations := []*Annotation{}
	var firstEntry *Entry

	if iterator, ok := iteratorT.(*Annotation); ok {
		allAnnotations = append(allAnnotations, iterator)
	}

	for {
		parentT, err := GetParentForEntry(repo, iteratorT)
		if err != nil {
			if errors.Is(err, ErrRSLEntryNotFound) {
				entry, ok := iteratorT.(*Entry)
				if !ok {
					// The first entry cannot be an annotation
					return nil, nil, ErrInvalidRSLEntry
				}
				firstEntry = entry
				break
			}

			return nil, nil, err
		}

		if annotation, ok := parentT.(*Annotation); ok {
			allAnnotations = append(allAnnotations, annotation)
		}

		iteratorT = parentT
	}

	annotations := filterAnnotationsForRelevantAnnotations(allAnnotations, firstEntry.ID)

	return firstEntry, annotations, nil
}

// GetFirstEntryForCommit returns the first entry in the RSL that either records
// the commit itself or a descendent of the commit. This establishes the first
// time a commit was seen in the repository, irrespective of the ref it was
// associated with, and we can infer things like the active developers who could
// have signed the commit.
func GetFirstEntryForCommit(repo *git.Repository, commit *object.Commit) (*Entry, []*Annotation, error) {
	// We check entries in pairs. In the initial case, we have the latest entry
	// and its parent. At all times, the parent in the pair is being tested.
	// If the latest entry is a descendant of the target commit, we start
	// checking the parent. The first pair where the parent entry is not
	// descended from the target commit, we return the other entry in the pair.

	firstEntry, firstAnnotations, err := GetLatestNonGittufEntry(repo)
	if err != nil {
		if errors.Is(err, ErrRSLEntryNotFound) {
			return nil, nil, ErrNoRecordOfCommit
		}
		return nil, nil, err
	}

	knowsCommit, err := gitinterface.KnowsCommit(repo, firstEntry.TargetID, commit)
	if err != nil {
		return nil, nil, err
	}
	if !knowsCommit {
		return nil, nil, ErrNoRecordOfCommit
	}

	for {
		iteratorEntry, iteratorAnnotations, err := GetNonGittufParentForEntry(repo, firstEntry)
		if err != nil {
			if errors.Is(err, ErrRSLEntryNotFound) {
				return firstEntry, firstAnnotations, nil
			}
			return nil, nil, err
		}

		knowsCommit, err := gitinterface.KnowsCommit(repo, iteratorEntry.TargetID, commit)
		if err != nil {
			return nil, nil, err
		}
		if !knowsCommit {
			return firstEntry, firstAnnotations, nil
		}

		firstEntry = iteratorEntry
		firstAnnotations = iteratorAnnotations
	}
}

// GetEntriesInRange returns a list of standard entries between the specified
// range and a map of annotations that refer to each standard entry in the
// range. The annotations map is keyed by the ID of the standard entry, with the
// value being a list of annotations that apply to that standard entry.
func GetEntriesInRange(repo *git.Repository, firstID, lastID plumbing.Hash) ([]*Entry, map[plumbing.Hash][]*Annotation, error) {
	return GetEntriesInRangeForRef(repo, firstID, lastID, "")
}

// GetEntriesInRangeForRef returns a list of standard entries for the ref
// between the specified range and a map of annotations that refer to each
// standard entry in the range. The annotations map is keyed by the ID of the
// standard entry, with the value being a list of annotations that apply to that
// standard entry.
func GetEntriesInRangeForRef(repo *git.Repository, firstID, lastID plumbing.Hash, refName string) ([]*Entry, map[plumbing.Hash][]*Annotation, error) {
	// We have to iterate from latest to get the annotations that refer to the
	// last requested entry
	iterator, err := GetLatestEntry(repo)
	if err != nil {
		return nil, nil, err
	}

	allAnnotations := []*Annotation{}
	for iterator.GetID() != lastID {
		// Until we find the entry corresponding to lastID, we just store
		// annotations
		if annotation, isAnnotation := iterator.(*Annotation); isAnnotation {
			allAnnotations = append(allAnnotations, annotation)
		}

		parent, err := GetParentForEntry(repo, iterator)
		if err != nil {
			return nil, nil, err
		}
		iterator = parent
	}

	entryStack := []*Entry{}
	inRange := map[plumbing.Hash]bool{}
	for iterator.GetID() != firstID {
		// Here, all items are relevant until the one corresponding to first is
		// found
		switch it := iterator.(type) {
		case *Entry:
			if len(refName) == 0 || it.RefName == refName || strings.HasPrefix(it.RefName, GittufNamespacePrefix) {
				// It's a relevant entry if:
				// a) there's no refName set, or
				// b) the entry's refName matches the set refName, or
				// c) the entry is for a gittuf namespace
				entryStack = append(entryStack, it)
				inRange[it.ID] = true
			}
		case *Annotation:
			allAnnotations = append(allAnnotations, it)
		}

		parent, err := GetParentForEntry(repo, iterator)
		if err != nil {
			return nil, nil, err
		}
		iterator = parent
	}

	// Handle the item corresponding to first explicitly
	// If it's an annotation, ignore it as it refers to something before the
	// range we care about
	if entry, isEntry := iterator.(*Entry); isEntry {
		if len(refName) == 0 || entry.RefName == refName || strings.HasPrefix(entry.RefName, GittufNamespacePrefix) {
			// It's a relevant entry if:
			// a) there's no refName set, or
			// b) the entry's refName matches the set refName, or
			// c) the entry is for a gittuf namespace
			entryStack = append(entryStack, entry)
			inRange[entry.ID] = true
		}
	}

	// For each annotation, add the entry to each relevant entry it refers to
	// Process annotations in reverse order so that annotations are listed in
	// order of occurrence in the map
	annotationMap := map[plumbing.Hash][]*Annotation{}
	for i := len(allAnnotations) - 1; i >= 0; i-- {
		annotation := allAnnotations[i]
		for _, entryID := range annotation.RSLEntryIDs {
			if _, relevant := inRange[entryID]; relevant {
				// Annotation is relevant because the entry it refers to was in
				// the specified range
				if _, exists := annotationMap[entryID]; !exists {
					annotationMap[entryID] = []*Annotation{}
				}

				annotationMap[entryID] = append(annotationMap[entryID], annotation)
			}
		}
	}

	// Reverse entryStack so that it's in order of occurrence rather than in
	// order of walking back the RSL
	allEntries := make([]*Entry, 0, len(entryStack))
	for i := len(entryStack) - 1; i >= 0; i-- {
		allEntries = append(allEntries, entryStack[i])
	}

	return allEntries, annotationMap, nil
}

func parseRSLEntryText(id plumbing.Hash, text string) (EntryType, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, AnnotationHeader) {
		return parseAnnotationText(id, text)
	}
	return parseEntryText(id, text)
}

func parseEntryText(id plumbing.Hash, text string) (*Entry, error) {
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		return nil, ErrInvalidRSLEntry
	}
	lines = lines[2:]

	entry := &Entry{ID: id}
	for _, l := range lines {
		l = strings.TrimSpace(l)

		ls := strings.Split(l, ":")
		if len(ls) < 2 {
			return nil, ErrInvalidRSLEntry
		}

		switch strings.TrimSpace(ls[0]) {
		case RefKey:
			entry.RefName = strings.TrimSpace(ls[1])
		case TargetIDKey:
			entry.TargetID = plumbing.NewHash(strings.TrimSpace(ls[1]))
		}
	}

	return entry, nil
}

func parseAnnotationText(id plumbing.Hash, text string) (*Annotation, error) {
	annotation := &Annotation{
		ID:          id,
		RSLEntryIDs: []plumbing.Hash{},
	}

	messageBlock, _ := pem.Decode([]byte(text)) // rest doesn't seem to work when the PEM block is at the end of text, see: https://go.dev/play/p/oZysAfemA-v
	if messageBlock != nil {
		annotation.Message = string(messageBlock.Bytes)
	}

	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		return nil, ErrInvalidRSLEntry
	}
	lines = lines[2:]

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == BeginMessage {
			break
		}

		ls := strings.Split(l, ":")
		if len(ls) < 2 {
			return nil, ErrInvalidRSLEntry
		}

		switch strings.TrimSpace(ls[0]) {
		case EntryIDKey:
			annotation.RSLEntryIDs = append(annotation.RSLEntryIDs, plumbing.NewHash(strings.TrimSpace(ls[1])))
		case SkipKey:
			if strings.TrimSpace(ls[1]) == "true" {
				annotation.Skip = true
			} else {
				annotation.Skip = false
			}
		}
	}

	return annotation, nil
}

func filterAnnotationsForRelevantAnnotations(allAnnotations []*Annotation, entryID plumbing.Hash) []*Annotation {
	annotations := []*Annotation{}
	for _, annotation := range allAnnotations {
		annotation := annotation
		if annotation.RefersTo(entryID) {
			annotations = append(annotations, annotation)
		}
	}

	if len(annotations) == 0 {
		return nil
	}

	return annotations
}
