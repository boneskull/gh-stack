// internal/state/state_test.go
package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/state"
)

func TestCascadeState(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)

	s := &state.CascadeState{
		Current:      "feature-b",
		Pending:      []string{"feature-c", "feature-d"},
		OriginalHead: "abc123",
	}

	err := state.Save(gitDir, s)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := state.Load(gitDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Current != s.Current {
		t.Errorf("Current mismatch: %q != %q", loaded.Current, s.Current)
	}
	if len(loaded.Pending) != len(s.Pending) {
		t.Errorf("Pending length mismatch")
	}
}

func TestCascadeStateNotExists(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)

	_, err := state.Load(gitDir)
	if err == nil {
		t.Error("expected error when state doesn't exist")
	}
}

func TestSubmitState(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	s := &state.CascadeState{
		Current:      "feature-b",
		Pending:      []string{"feature-c"},
		OriginalHead: "abc123",
		Operation:    state.OperationSubmit,
		UpdateOnly:   true,
	}

	if err := state.Save(gitDir, s); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := state.Load(gitDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Operation != state.OperationSubmit {
		t.Errorf("expected operation %q, got %q", state.OperationSubmit, loaded.Operation)
	}
	if !loaded.UpdateOnly {
		t.Error("expected UpdateOnly to be true")
	}
}
