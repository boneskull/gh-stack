// internal/github/github.go
package github

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// PR represents a GitHub pull request.
type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
}

// Comment represents a GitHub issue/PR comment.
type Comment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// Client wraps the go-gh REST client with repo context.
type Client struct {
	rest  *api.RESTClient
	owner string
	repo  string
}

// NewClient creates a new GitHub client for the current repository.
func NewClient() (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	repo, err := repository.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to detect repository: %w", err)
	}

	return &Client{
		rest:  rest,
		owner: repo.Owner,
		repo:  repo.Name,
	}, nil
}

// CreatePR creates a new pull request and returns the PR number.
func (c *Client) CreatePR(head, base, title, body string) (int, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls", c.owner, c.repo)

	request := struct {
		Head  string `json:"head"`
		Base  string `json:"base"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}{
		Head:  head,
		Base:  base,
		Title: title,
		Body:  body,
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	var response PR
	err = c.rest.Post(path, bytes.NewReader(reqBody), &response)
	if err != nil {
		return 0, fmt.Errorf("failed to create PR: %w", err)
	}

	return response.Number, nil
}

// GetPR fetches PR details by number.
func (c *Client) GetPR(number int) (*PR, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, number)

	var pr PR
	err := c.rest.Get(path, &pr)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR #%d: %w", number, err)
	}

	return &pr, nil
}

// UpdatePRBase updates the base branch of a PR.
func (c *Client) UpdatePRBase(number int, base string) error {
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, number)

	request := struct {
		Base string `json:"base"`
	}{
		Base: base,
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.rest.Patch(path, bytes.NewReader(reqBody), nil)
	if err != nil {
		return fmt.Errorf("failed to update PR #%d base: %w", number, err)
	}

	return nil
}

// CreateComment adds a comment to a PR (PRs are issues in GitHub's API).
func (c *Client) CreateComment(prNumber int, body string) (int, error) {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", c.owner, c.repo, prNumber)

	request := struct {
		Body string `json:"body"`
	}{
		Body: body,
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal comment body: %w", err)
	}

	var response Comment
	err = c.rest.Post(path, bytes.NewReader(reqBody), &response)
	if err != nil {
		return 0, fmt.Errorf("failed to create comment on PR #%d: %w", prNumber, err)
	}

	return response.ID, nil
}

// ListComments retrieves all comments on a PR.
func (c *Client) ListComments(prNumber int) ([]Comment, error) {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", c.owner, c.repo, prNumber)

	var comments []Comment
	if err := c.rest.Get(path, &comments); err != nil {
		return nil, fmt.Errorf("list comments on PR #%d: %w", prNumber, err)
	}

	return comments, nil
}

// UpdateComment updates an existing comment by ID.
func (c *Client) UpdateComment(commentID int, body string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/comments/%d", c.owner, c.repo, commentID)

	request := struct {
		Body string `json:"body"`
	}{
		Body: body,
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal comment body: %w", err)
	}

	if err := c.rest.Patch(path, bytes.NewReader(reqBody), nil); err != nil {
		return fmt.Errorf("update comment %d: %w", commentID, err)
	}

	return nil
}

// --- Convenience functions for backward compatibility ---

// defaultClient is a lazily-initialized client for convenience functions.
var defaultClient *Client

func getDefaultClient() (*Client, error) {
	if defaultClient == nil {
		var err error
		defaultClient, err = NewClient()
		if err != nil {
			return nil, err
		}
	}
	return defaultClient, nil
}

// CreatePR creates a new pull request using the default client.
// Deprecated: Use NewClient() and call methods directly for better error handling.
func CreatePR(head, base, title, body string) (int, error) {
	client, err := getDefaultClient()
	if err != nil {
		return 0, err
	}
	return client.CreatePR(head, base, title, body)
}

// GetPR fetches PR details using the default client.
func GetPR(number int) (*PR, error) {
	client, err := getDefaultClient()
	if err != nil {
		return nil, err
	}
	return client.GetPR(number)
}

// UpdatePRBase updates the base branch using the default client.
func UpdatePRBase(number int, base string) error {
	client, err := getDefaultClient()
	if err != nil {
		return err
	}
	return client.UpdatePRBase(number, base)
}
