// internal/undo/undo.go
package undo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	undoDir    = "stack-undo"
	archiveDir = "done"
	timeFormat = "20060102T150405.000000000Z" // Compact ISO8601 with nanoseconds to avoid collisions

	// maxActiveSnapshots is the maximum number of pending undo snapshots to keep.
	// Oldest snapshots are pruned automatically when this limit is exceeded.
	maxActiveSnapshots = 50

	// maxArchivedSnapshots is the maximum number of archived (used) snapshots to keep.
	// Oldest archived snapshots are pruned automatically when this limit is exceeded.
	maxArchivedSnapshots = 50
)

// ErrNoSnapshot is returned when no undo snapshot exists.
var ErrNoSnapshot = errors.New("nothing to undo")

// BranchState captures the state of a single branch for undo.
type BranchState struct {
	SHA            string `json:"sha"`
	StackParent    string `json:"stack_parent,omitempty"`
	StackPR        int    `json:"stack_pr,omitempty"`
	StackForkPoint string `json:"stack_fork_point,omitempty"`
}

// Snapshot represents the complete state before a destructive operation.
type Snapshot struct {
	Timestamp       time.Time              `json:"timestamp"`
	Operation       string                 `json:"operation"`
	Command         string                 `json:"command"`
	OriginalHead    string                 `json:"original_head"`
	StashRef        string                 `json:"stash_ref,omitempty"`
	Branches        map[string]BranchState `json:"branches"`
	DeletedBranches map[string]BranchState `json:"deleted_branches,omitempty"`
}

// NewSnapshot creates a new snapshot with the current timestamp.
func NewSnapshot(operation, command, originalHead string) *Snapshot {
	return &Snapshot{
		Timestamp:       time.Now().UTC(),
		Operation:       operation,
		Command:         command,
		OriginalHead:    originalHead,
		Branches:        make(map[string]BranchState),
		DeletedBranches: make(map[string]BranchState),
	}
}

// Save persists the snapshot to .git/stack-undo/{timestamp}-{operation}.json.
// Automatically prunes old snapshots if the count exceeds maxActiveSnapshots.
func Save(gitDir string, snapshot *Snapshot) error {
	dir := filepath.Join(gitDir, undoDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := snapshot.Timestamp.Format(timeFormat) + "-" + snapshot.Operation + ".json"
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// Prune old snapshots if we exceed the limit
	return pruneDir(dir, maxActiveSnapshots)
}

// LoadLatest reads the most recent snapshot from .git/stack-undo/.
// Returns the snapshot, its file path, and any error.
func LoadLatest(gitDir string) (*Snapshot, string, error) {
	dir := filepath.Join(gitDir, undoDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrNoSnapshot
		}
		return nil, "", err
	}

	// Filter to only .json files (not the done/ directory)
	var jsonFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles = append(jsonFiles, entry)
		}
	}

	if len(jsonFiles) == 0 {
		return nil, "", ErrNoSnapshot
	}

	// Sort by name (timestamp prefix) descending to get most recent
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() > jsonFiles[j].Name()
	})

	path := filepath.Join(dir, jsonFiles[0].Name())
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, "", err
	}
	return &snapshot, path, nil
}

// Load reads a snapshot from a specific path.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// Archive moves a snapshot file to the done/ subdirectory.
// Automatically prunes old archived snapshots if the count exceeds maxArchivedSnapshots.
func Archive(gitDir, snapshotPath string) error {
	archivePath := filepath.Join(gitDir, undoDir, archiveDir)
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return err
	}

	filename := filepath.Base(snapshotPath)
	dest := filepath.Join(archivePath, filename)
	if err := os.Rename(snapshotPath, dest); err != nil {
		return err
	}

	// Prune old archived snapshots if we exceed the limit
	return pruneDir(archivePath, maxArchivedSnapshots)
}

// List returns all available (non-archived) snapshots, sorted newest first.
func List(gitDir string) ([]*Snapshot, error) {
	dir := filepath.Join(gitDir, undoDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Filter to only .json files
	var jsonFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles = append(jsonFiles, entry)
		}
	}

	// Sort by name (timestamp prefix) descending
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() > jsonFiles[j].Name()
	})

	var snapshots []*Snapshot
	for _, file := range jsonFiles {
		path := filepath.Join(dir, file.Name())
		snapshot, err := Load(path)
		if err != nil {
			continue // Skip malformed files
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// Exists checks if any undo snapshots exist.
func Exists(gitDir string) bool {
	dir := filepath.Join(gitDir, undoDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			return true
		}
	}
	return false
}

// Remove deletes a snapshot file.
func Remove(snapshotPath string) error {
	err := os.Remove(snapshotPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// pruneDir removes the oldest .json files in dir if the count exceeds max.
// Files are sorted by name (which starts with a timestamp), so oldest are deleted first.
func pruneDir(dir string, max int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Filter to only .json files
	var jsonFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles = append(jsonFiles, entry)
		}
	}

	// Nothing to prune
	if len(jsonFiles) <= max {
		return nil
	}

	// Sort ascending by name (oldest first, since filenames start with timestamp)
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() < jsonFiles[j].Name()
	})

	// Delete oldest files until we're at or below max
	toDelete := len(jsonFiles) - max
	for i := 0; i < toDelete; i++ {
		path := filepath.Join(dir, jsonFiles[i].Name())
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
	}

	return nil
}
