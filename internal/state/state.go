// internal/state/state.go
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const stateFile = "STACK_CASCADE_STATE"

// Operation types for restack state.
const (
	OperationRestack = "restack"
	OperationSubmit  = "submit"
)

// ErrNoState is returned when no restack state exists.
var ErrNoState = errors.New("no restack in progress")

// RestackState represents the state of an in-progress restack or submit operation.
type RestackState struct {
	Current      string   `json:"current"`
	Pending      []string `json:"pending"`
	OriginalHead string   `json:"original_head"`
	// Operation is "restack" or "submit" - determines what happens after restack completes
	Operation string `json:"operation,omitempty"`
	// UpdateOnly (submit only) - if true, don't create new PRs, only update existing
	UpdateOnly bool `json:"update_only,omitempty"`
	// Web (submit only) - if true, open PRs in browser after creation/update
	Web bool `json:"web,omitempty"`
	// PushOnly (submit only) - if true, skip PR creation/update phase entirely
	PushOnly bool `json:"push_only,omitempty"`
	// Branches (submit only) - the complete list of branches being submitted.
	// Used to rebuild the full set for push/PR phases after restack completes.
	Branches []string `json:"branches,omitempty"`
	// StashRef is the commit hash of auto-stashed changes (if any).
	// Used to restore working tree changes when operation completes or is aborted.
	StashRef string `json:"stash_ref,omitempty"`
	// Worktrees maps branch names to linked worktree paths.
	// Persisted so that continue/abort can find the correct worktree
	// directory for branches that were being rebased in a linked worktree.
	Worktrees map[string]string `json:"worktrees,omitempty"`
}

// Save persists restack state to .git/STACK_CASCADE_STATE.
func Save(gitDir string, s *RestackState) error {
	path := filepath.Join(gitDir, stateFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads restack state from .git/STACK_CASCADE_STATE.
func Load(gitDir string) (*RestackState, error) {
	path := filepath.Join(gitDir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoState
		}
		return nil, err
	}

	var s RestackState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Remove deletes the restack state file.
func Remove(gitDir string) error {
	path := filepath.Join(gitDir, stateFile)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Exists checks if a restack is in progress.
func Exists(gitDir string) bool {
	path := filepath.Join(gitDir, stateFile)
	_, err := os.Stat(path)
	return err == nil
}
