// internal/config/config_test.go
package config_test

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user for commits
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	return dir
}

func TestGetTrunk_NotInitialized(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, err = cfg.GetTrunk()
	if err == nil {
		t.Error("expected error when trunk not set, got nil")
	}
}

func TestSetTrunk(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := cfg.SetTrunk("main"); err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		t.Fatalf("GetTrunk failed: %v", err)
	}
	if trunk != "main" {
		t.Errorf("expected trunk 'main', got %q", trunk)
	}
}

func TestGetParent_NotTracked(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)

	_, err := cfg.GetParent("feature-a")
	if !errors.Is(err, config.ErrBranchNotTracked) {
		t.Errorf("expected ErrBranchNotTracked, got %v", err)
	}
}

func TestSetAndGetParent(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")

	if err := cfg.SetParent("feature-a", "main"); err != nil {
		t.Fatalf("SetParent failed: %v", err)
	}

	parent, err := cfg.GetParent("feature-a")
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent != "main" {
		t.Errorf("expected parent 'main', got %q", parent)
	}
}
