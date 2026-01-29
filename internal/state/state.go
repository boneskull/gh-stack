// internal/state/state.go
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const stateFile = "STACK_CASCADE_STATE"

// Operation types for cascade state.
const (
	OperationCascade = "cascade"
	OperationSubmit  = "submit"
)

// ErrNoState is returned when no cascade state exists.
var ErrNoState = errors.New("no cascade in progress")

// CascadeState represents the state of an in-progress cascade or submit operation.
type CascadeState struct {
	Current      string   `json:"current"`
	Pending      []string `json:"pending"`
	OriginalHead string   `json:"original_head"`
	// Operation is "cascade" or "submit" - determines what happens after cascade completes
	Operation string `json:"operation,omitempty"`
	// UpdateOnly (submit only) - if true, don't create new PRs, only update existing
	UpdateOnly bool `json:"update_only,omitempty"`
}

// Save persists cascade state to .git/STACK_CASCADE_STATE.
func Save(gitDir string, s *CascadeState) error {
	path := filepath.Join(gitDir, stateFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads cascade state from .git/STACK_CASCADE_STATE.
func Load(gitDir string) (*CascadeState, error) {
	path := filepath.Join(gitDir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoState
		}
		return nil, err
	}

	var s CascadeState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Remove deletes the cascade state file.
func Remove(gitDir string) error {
	path := filepath.Join(gitDir, stateFile)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Exists checks if a cascade is in progress.
func Exists(gitDir string) bool {
	path := filepath.Join(gitDir, stateFile)
	_, err := os.Stat(path)
	return err == nil
}
