// internal/github/github.go
package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PR represents a GitHub pull request.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
}

// CreatePR creates a new pull request and returns the PR number.
func CreatePR(base, title, body string) (int, error) {
	args := []string{"pr", "create", "--base", base, "--title", title, "--body", body}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return 0, fmt.Errorf("gh pr create failed: %s", string(exitErr.Stderr))
		}
		return 0, fmt.Errorf("gh pr create failed: %w", err)
	}

	// Output is the PR URL, extract the number
	url := strings.TrimSpace(string(out))
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("unexpected output: %s", url)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

// GetPR fetches PR details by number.
func GetPR(number int) (*PR, error) {
	out, err := exec.Command("gh", "pr", "view", strconv.Itoa(number), "--json", "number,state,merged").Output()
	if err != nil {
		return nil, err
	}

	var pr PR
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// UpdatePRBase updates the base branch of a PR.
func UpdatePRBase(number int, base string) error {
	return exec.Command("gh", "pr", "edit", strconv.Itoa(number), "--base", base).Run()
}
