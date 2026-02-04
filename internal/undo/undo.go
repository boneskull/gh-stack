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
	timeFormat = "20060102T150405Z" // Compact ISO8601 for filenames
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
func Save(gitDir string, s *Snapshot) error {
	dir := filepath.Join(gitDir, undoDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := s.Timestamp.Format(timeFormat) + "-" + s.Operation + ".json"
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
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

	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, "", err
	}
	return &s, path, nil
}

// Load reads a snapshot from a specific path.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Archive moves a snapshot file to the done/ subdirectory.
func Archive(gitDir, snapshotPath string) error {
	archivePath := filepath.Join(gitDir, undoDir, archiveDir)
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return err
	}

	filename := filepath.Base(snapshotPath)
	dest := filepath.Join(archivePath, filename)
	return os.Rename(snapshotPath, dest)
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
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		}
	}

	// Sort by name (timestamp prefix) descending
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() > jsonFiles[j].Name()
	})

	var snapshots []*Snapshot
	for _, f := range jsonFiles {
		path := filepath.Join(dir, f.Name())
		s, err := Load(path)
		if err != nil {
			continue // Skip malformed files
		}
		snapshots = append(snapshots, s)
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
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
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
