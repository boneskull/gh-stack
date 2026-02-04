// internal/undo/undo_test.go
package undo_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/boneskull/gh-stack/internal/undo"
)

func TestSaveAndLoadLatest(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	snapshot := undo.NewSnapshot("cascade", "gh stack cascade", "feature-a")
	snapshot.Branches["feature-a"] = undo.BranchState{
		SHA:         "abc123def456",
		StackParent: "main",
		StackPR:     42,
	}
	snapshot.Branches["feature-b"] = undo.BranchState{
		SHA:            "789xyz000111",
		StackParent:    "feature-a",
		StackForkPoint: "abc123def456",
	}

	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, path, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatalf("LoadLatest failed: %v", err)
	}

	if loaded.Operation != "cascade" {
		t.Errorf("Operation mismatch: %q != %q", loaded.Operation, "cascade")
	}
	if loaded.Command != "gh stack cascade" {
		t.Errorf("Command mismatch: %q != %q", loaded.Command, "gh stack cascade")
	}
	if loaded.OriginalHead != "feature-a" {
		t.Errorf("OriginalHead mismatch: %q != %q", loaded.OriginalHead, "feature-a")
	}
	if len(loaded.Branches) != 2 {
		t.Errorf("Expected 2 branches, got %d", len(loaded.Branches))
	}

	// Check branch state
	featureA, ok := loaded.Branches["feature-a"]
	if !ok {
		t.Error("feature-a not found in branches")
	} else {
		if featureA.SHA != "abc123def456" {
			t.Errorf("feature-a SHA mismatch: %q", featureA.SHA)
		}
		if featureA.StackPR != 42 {
			t.Errorf("feature-a StackPR mismatch: %d", featureA.StackPR)
		}
	}

	// Verify path is not empty
	if path == "" {
		t.Error("LoadLatest returned empty path")
	}
}

func TestLoadLatestReturnsNewest(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create first snapshot
	snapshot1 := undo.NewSnapshot("cascade", "gh stack cascade", "feature-a")
	snapshot1.Timestamp = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	snapshot1.Branches["feature-a"] = undo.BranchState{SHA: "first"}
	if err := undo.Save(gitDir, snapshot1); err != nil {
		t.Fatal(err)
	}

	// Create second snapshot (newer)
	snapshot2 := undo.NewSnapshot("submit", "gh stack submit", "feature-b")
	snapshot2.Timestamp = time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	snapshot2.Branches["feature-b"] = undo.BranchState{SHA: "second"}
	if err := undo.Save(gitDir, snapshot2); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should return the newer snapshot
	if loaded.Operation != "submit" {
		t.Errorf("Expected submit (newer), got %q", loaded.Operation)
	}
}

func TestNoSnapshot(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, _, err := undo.LoadLatest(gitDir)
	if err != undo.ErrNoSnapshot {
		t.Errorf("Expected ErrNoSnapshot, got %v", err)
	}
}

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	snapshot := undo.NewSnapshot("cascade", "gh stack cascade", "main")
	snapshot.Branches["feature"] = undo.BranchState{SHA: "abc123"}
	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatal(err)
	}

	// Load to get the path
	_, path, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	// Archive it
	if archiveErr := undo.Archive(gitDir, path); archiveErr != nil {
		t.Fatalf("Archive failed: %v", archiveErr)
	}

	// Original should no longer exist
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("Original file should be deleted after archive")
	}

	// Should be in done/ directory
	doneDir := filepath.Join(gitDir, "stack-undo", "done")
	entries, err := os.ReadDir(doneDir)
	if err != nil {
		t.Fatalf("Failed to read done dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("Expected 1 file in done/, got %d", len(entries))
	}

	// LoadLatest should now return ErrNoSnapshot
	_, _, err = undo.LoadLatest(gitDir)
	if err != undo.ErrNoSnapshot {
		t.Errorf("Expected ErrNoSnapshot after archive, got %v", err)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple snapshots
	for i, op := range []string{"cascade", "submit", "sync"} {
		snapshot := undo.NewSnapshot(op, "gh stack "+op, "main")
		snapshot.Timestamp = time.Date(2024, 1, i+1, 12, 0, 0, 0, time.UTC)
		snapshot.Branches["feature"] = undo.BranchState{SHA: "abc"}
		if err := undo.Save(gitDir, snapshot); err != nil {
			t.Fatal(err)
		}
	}

	snapshots, err := undo.List(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Should be sorted newest first
	if snapshots[0].Operation != "sync" {
		t.Errorf("Expected sync (newest) first, got %q", snapshots[0].Operation)
	}
	if snapshots[2].Operation != "cascade" {
		t.Errorf("Expected cascade (oldest) last, got %q", snapshots[2].Operation)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	if undo.Exists(gitDir) {
		t.Error("Exists should return false when no snapshots")
	}

	snapshot := undo.NewSnapshot("cascade", "gh stack cascade", "main")
	snapshot.Branches["feature"] = undo.BranchState{SHA: "abc"}
	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatal(err)
	}

	if !undo.Exists(gitDir) {
		t.Error("Exists should return true after saving snapshot")
	}
}

func TestDeletedBranches(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	snapshot := undo.NewSnapshot("sync", "gh stack sync", "main")
	snapshot.Branches["feature-a"] = undo.BranchState{
		SHA:         "abc123",
		StackParent: "main",
	}
	snapshot.DeletedBranches["feature-merged"] = undo.BranchState{
		SHA:         "deleted123",
		StackParent: "main",
		StackPR:     99,
	}

	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.DeletedBranches) != 1 {
		t.Errorf("Expected 1 deleted branch, got %d", len(loaded.DeletedBranches))
	}

	deleted, ok := loaded.DeletedBranches["feature-merged"]
	if !ok {
		t.Error("feature-merged not found in deleted branches")
	} else {
		if deleted.SHA != "deleted123" {
			t.Errorf("Deleted branch SHA mismatch: %q", deleted.SHA)
		}
		if deleted.StackPR != 99 {
			t.Errorf("Deleted branch StackPR mismatch: %d", deleted.StackPR)
		}
	}
}

func TestStashRef(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	snapshot := undo.NewSnapshot("cascade", "gh stack cascade", "main")
	snapshot.StashRef = "stash@{0}"
	snapshot.Branches["feature"] = undo.BranchState{SHA: "abc"}

	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatal(err)
	}

	loaded, _, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.StashRef != "stash@{0}" {
		t.Errorf("StashRef mismatch: %q", loaded.StashRef)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	snapshot := undo.NewSnapshot("cascade", "gh stack cascade", "main")
	snapshot.Branches["feature"] = undo.BranchState{SHA: "abc"}
	if err := undo.Save(gitDir, snapshot); err != nil {
		t.Fatal(err)
	}

	_, path, err := undo.LoadLatest(gitDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := undo.Remove(path); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if undo.Exists(gitDir) {
		t.Error("Snapshot should not exist after Remove")
	}

	// Remove non-existent should not error
	if err := undo.Remove(path); err != nil {
		t.Errorf("Remove non-existent should not error: %v", err)
	}
}
