// cmd/init_test.go
package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/git"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Run()
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create main branch with initial commit
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# Test"), 0644)
	run("add", ".")
	run("commit", "-m", "initial")

	return dir
}

func TestInitCommand(t *testing.T) {
	dir := setupTestRepo(t)

	// Verify the config package works for init
	g := git.New(dir)
	cfg, err := config.New(g)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	err = cfg.SetTrunk("main")
	if err != nil {
		t.Fatalf("SetTrunk failed: %v", err)
	}

	trunk, err := cfg.GetTrunk()
	if err != nil {
		t.Fatalf("GetTrunk failed: %v", err)
	}
	if trunk != "main" {
		t.Errorf("expected 'main', got %q", trunk)
	}
}
