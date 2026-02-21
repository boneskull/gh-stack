// cmd/log_test.go
package cmd_test

import (
	"strings"
	"testing"

	"github.com/boneskull/gh-stack/internal/config"
	"github.com/boneskull/gh-stack/internal/tree"
)

func TestLogBuildTree(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "feature-a")
	cfg.SetPR("feature-a", 123)

	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify tree structure
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
	if featureA.PR != 123 {
		t.Errorf("expected PR 123, got %d", featureA.PR)
	}
	if len(featureA.Children) != 1 {
		t.Fatalf("expected 1 child of feature-a, got %d", len(featureA.Children))
	}

	featureB := featureA.Children[0]
	if featureB.Name != "feature-b" {
		t.Errorf("expected 'feature-b', got %q", featureB.Name)
	}
}

func TestLogEmptyStack(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")

	// No branches tracked, just trunk
	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if root.Name != "main" {
		t.Errorf("expected root 'main', got %q", root.Name)
	}
	if len(root.Children) != 0 {
		t.Errorf("expected 0 children for trunk-only stack, got %d", len(root.Children))
	}
}

func TestLogMultipleBranches(t *testing.T) {
	dir := setupTestRepo(t)

	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")
	// Two branches off main
	cfg.SetParent("feature-a", "main")
	cfg.SetParent("feature-b", "main")
	// One branch off feature-a
	cfg.SetParent("feature-a-sub", "feature-a")

	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(root.Children) != 2 {
		t.Errorf("expected 2 children of main, got %d", len(root.Children))
	}

	// Find feature-a and check its children
	var featureA *tree.Node
	for _, child := range root.Children {
		if child.Name == "feature-a" {
			featureA = child
			break
		}
	}

	if featureA == nil {
		t.Fatal("feature-a not found")
		return
	}

	if len(featureA.Children) != 1 {
		t.Errorf("expected 1 child of feature-a, got %d", len(featureA.Children))
	}
	if featureA.Children[0].Name != "feature-a-sub" {
		t.Errorf("expected 'feature-a-sub', got %q", featureA.Children[0].Name)
	}
}

func TestLogTreeWithDetectedNode(t *testing.T) {
	dir := setupTestRepo(t)
	cfg, _ := config.Load(dir)
	cfg.SetTrunk("main")
	cfg.SetParent("feature-a", "main")

	root, err := tree.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Manually inject a detected node (simulating what log would do)
	featureA := tree.FindNode(root, "feature-a")
	if featureA == nil {
		t.Fatal("feature-a not found")
	}

	detected := &tree.Node{
		Name:       "feature-b",
		Parent:     featureA,
		Detected:   true,
		Confidence: tree.ConfidenceMedium,
	}
	featureA.Children = append(featureA.Children, detected)

	// Verify FormatTree includes the detected node with annotation
	output := tree.FormatTree(root, tree.FormatOptions{})
	if !strings.Contains(output, "feature-b") {
		t.Errorf("expected output to contain 'feature-b', got:\n%s", output)
	}
	if !strings.Contains(output, "detected") {
		t.Errorf("expected output to contain 'detected' annotation, got:\n%s", output)
	}
}
