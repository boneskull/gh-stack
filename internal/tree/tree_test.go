// internal/tree/tree_test.go
package tree_test

import (
	"os/exec"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/tree"
)

func setupTestRepo(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	cfg, _ := config.Load(dir)
	return cfg, dir
}

func TestBuildTree(t *testing.T) {
	cfg, _ := setupTestRepo(t)

	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")
	cfg.SetParent("feature-c", "feature-a")

	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if root.Name != "main" {
		t.Errorf("expected root 'main', got %q", root.Name)
	}

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child of main, got %d", len(root.Children))
	}

	featureA := root.Children[0]
	if featureA.Name != "feature-a" {
		t.Errorf("expected 'feature-a', got %q", featureA.Name)
	}

	if len(featureA.Children) != 2 {
		t.Errorf("expected 2 children of feature-a, got %d", len(featureA.Children))
	}
}

func TestFindNode(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")

	root, _ := tree.Build(cfg)

	node := tree.FindNode(root, "feature-a")
	if node == nil {
		t.Fatal("FindNode returned nil")
	}
	if node.Name != "feature-a" {
		t.Errorf("expected 'feature-a', got %q", node.Name)
	}

	notFound := tree.FindNode(root, "nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent branch")
	}
}

func TestGetAncestors(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")

	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-b")

	ancestors := tree.GetAncestors(node)
	if len(ancestors) != 2 {
		t.Fatalf("expected 2 ancestors, got %d", len(ancestors))
	}
	if ancestors[0].Name != "feature-a" || ancestors[1].Name != "main" {
		t.Errorf("unexpected ancestors: %v", ancestors)
	}
}

func TestGetDescendants(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")
	cfg.SetParent("feature-c", "feature-a")
	cfg.SetParent("feature-d", "feature-b")

	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-a")

	descendants := tree.GetDescendants(node)

	// Should get feature-b, feature-c, feature-d in depth-first order
	if len(descendants) != 3 {
		t.Fatalf("expected 3 descendants, got %d", len(descendants))
	}

	// Verify all expected descendants are present
	names := make(map[string]bool)
	for _, d := range descendants {
		names[d.Name] = true
	}
	for _, expected := range []string{"feature-b", "feature-c", "feature-d"} {
		if !names[expected] {
			t.Errorf("missing descendant: %s", expected)
		}
	}
}

func TestGetDescendantsEmpty(t *testing.T) {
	cfg, _ := setupTestRepo(t)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")

	root, _ := tree.Build(cfg)
	node := tree.FindNode(root, "feature-a")

	descendants := tree.GetDescendants(node)
	if len(descendants) != 0 {
		t.Errorf("expected 0 descendants for leaf node, got %d", len(descendants))
	}
}

func TestFormatTree(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*config.Config)
		current  string
		prURL    func(int) string
		expected string
	}{
		{
			name: "single branch",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
			},
			expected: "main\n└── feature-a\n",
		},
		{
			name: "linear stack",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetParent("feature-b", "feature-a")
			},
			expected: "main\n└── feature-a\n    └── feature-b\n",
		},
		{
			name: "branching stack",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetParent("feature-b", "main")
			},
			expected: "main\n├── feature-a\n└── feature-b\n",
		},
		{
			name: "complex tree",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetParent("feature-b", "main")
				cfg.SetParent("feature-a-sub", "feature-a")
			},
			expected: "main\n├── feature-a\n│   └── feature-a-sub\n└── feature-b\n",
		},
		{
			name: "current branch marker",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
			},
			current:  "feature-a",
			expected: "main\n└── * feature-a\n",
		},
		{
			name: "current branch is trunk",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
			},
			current:  "main",
			expected: "* main\n└── feature-a\n",
		},
		{
			name: "with PR number",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetPR("feature-a", 42)
			},
			expected: "main\n└── feature-a (#42)\n",
		},
		{
			name: "with PR URL",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetPR("feature-a", 42)
			},
			prURL: func(pr int) string {
				return "https://github.com/owner/repo/pull/42"
			},
			expected: "main\n└── feature-a (#42) https://github.com/owner/repo/pull/42\n",
		},
		{
			name: "trunk only",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
			},
			expected: "main\n",
		},
		{
			name: "deep nesting",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("a", "main")
				cfg.SetParent("b", "a")
				cfg.SetParent("c", "b")
			},
			expected: "main\n└── a\n    └── b\n        └── c\n",
		},
		{
			name: "mixed siblings and children",
			setup: func(cfg *config.Config) {
				cfg.SetTrunk("main")
				cfg.SetParent("feature-a", "main")
				cfg.SetParent("feature-b", "main")
				cfg.SetParent("feature-a1", "feature-a")
				cfg.SetParent("feature-a2", "feature-a")
			},
			expected: "main\n├── feature-a\n│   ├── feature-a1\n│   └── feature-a2\n└── feature-b\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := setupTestRepo(t)
			tt.setup(cfg)

			root, err := tree.Build(cfg)
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}

			opts := tree.FormatOptions{
				CurrentBranch: tt.current,
				PRURLFunc:     tt.prURL,
			}

			got := tree.FormatTree(root, opts)
			if got != tt.expected {
				t.Errorf("FormatTree mismatch:\ngot:\n%s\nexpected:\n%s", got, tt.expected)
			}
		})
	}
}
