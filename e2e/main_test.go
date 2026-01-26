// e2e/main_test.go
package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// binaryPath holds the path to the compiled gh-stack binary.
var binaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "gh-stack-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmpDir, "gh-stack")

	// Find project root (parent of e2e/)
	projectRoot, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve project root: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	ldflags := `-ldflags=-X github.com/boneskull/gh-stack/cmd.version=e2e-test`
	cmd := exec.Command("go", "build", ldflags, "-o", binaryPath, ".")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}
