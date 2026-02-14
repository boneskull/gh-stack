// internal/config/config_test.go
package config_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	if setErr := cfg.SetTrunk("main"); setErr != nil {
		t.Fatalf("SetTrunk failed: %v", setErr)
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

func TestPRNumber(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)

	// No PR set initially
	_, err := cfg.GetPR("feature-a")
	if err == nil {
		t.Error("expected error for unset PR, got nil")
	}

	// Set PR
	if setErr := cfg.SetPR("feature-a", 1234); setErr != nil {
		t.Fatalf("SetPR failed: %v", setErr)
	}

	// Get PR
	pr, getErr := cfg.GetPR("feature-a")
	if getErr != nil {
		t.Fatalf("GetPR failed: %v", getErr)
	}
	if pr != 1234 {
		t.Errorf("expected PR 1234, got %d", pr)
	}

	// Remove PR
	if removeErr := cfg.RemovePR("feature-a"); removeErr != nil {
		t.Fatalf("RemovePR failed: %v", removeErr)
	}

	_, err = cfg.GetPR("feature-a")
	if err == nil {
		t.Error("expected error after removing PR, got nil")
	}
}

func TestListTrackedBranches(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")

	branches, err := cfg.ListTrackedBranches()
	if err != nil {
		t.Fatalf("ListTrackedBranches failed: %v", err)
	}

	if len(branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(branches))
	}

	// Check both branches are present
	found := make(map[string]bool)
	for _, b := range branches {
		found[b] = true
	}
	if !found["feature-a"] || !found["feature-b"] {
		t.Errorf("missing expected branches, got %v", branches)
	}
}

func TestForkPoint(t *testing.T) {
	dir := setupTestRepo(t)
	cfg, _ := config.Load(dir)

	// Initially no fork point
	_, err := cfg.GetForkPoint("feature")
	if !errors.Is(err, config.ErrNoForkPoint) {
		t.Errorf("GetForkPoint = %v, want ErrNoForkPoint", err)
	}

	// Set fork point
	sha := "abc123def456"
	if setErr := cfg.SetForkPoint("feature", sha); setErr != nil {
		t.Fatalf("SetForkPoint failed: %v", setErr)
	}

	// Get fork point
	got, err := cfg.GetForkPoint("feature")
	if err != nil {
		t.Fatalf("GetForkPoint failed: %v", err)
	}
	if got != sha {
		t.Errorf("GetForkPoint = %q, want %q", got, sha)
	}

	// Remove fork point
	if removeErr := cfg.RemoveForkPoint("feature"); removeErr != nil {
		t.Fatalf("RemoveForkPoint failed: %v", removeErr)
	}

	// Verify removed
	_, err = cfg.GetForkPoint("feature")
	if !errors.Is(err, config.ErrNoForkPoint) {
		t.Errorf("after remove, GetForkPoint = %v, want ErrNoForkPoint", err)
	}
}

func TestSetForkPointWithComment(t *testing.T) {
	dir := setupTestRepo(t)
	cfg, _ := config.Load(dir)

	sha := "abc123def456"
	comment := "replaces deadbee (2026-02-14T00:00:00Z)"

	if err := cfg.SetForkPointWithComment("feature", sha, comment); err != nil {
		t.Fatalf("SetForkPointWithComment failed: %v", err)
	}

	// Value should be readable via GetForkPoint (git config ignores inline comments)
	got, err := cfg.GetForkPoint("feature")
	if err != nil {
		t.Fatalf("GetForkPoint after SetForkPointWithComment failed: %v", err)
	}
	if got != sha {
		t.Errorf("GetForkPoint = %q, want %q", got, sha)
	}

	// The raw config file should contain the inline comment
	configPath := filepath.Join(dir, ".git", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read .git/config: %v", err)
	}
	configStr := string(data)
	if !strings.Contains(configStr, "# "+comment) {
		t.Errorf("expected inline comment in config, got:\n%s", configStr)
	}
}
