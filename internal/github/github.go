// internal/github/github.go
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// RESTClient defines the interface for GitHub REST API operations.
// The *api.RESTClient from go-gh satisfies this interface.
type RESTClient interface {
	Get(path string, response any) error
	Post(path string, body io.Reader, response any) error
	Patch(path string, body io.Reader, response any) error
}

// PR represents a GitHub pull request.
type PR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Draft  bool   `json:"draft"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// PRInfo contains display information for a PR in stack comments.
type PRInfo struct {
	Number int
	Title  string
}

// GetPRTitles fetches titles for multiple PRs in a single GraphQL request.
// Returns a map of PR number to PRInfo. PRs that don't exist are omitted.
func (c *Client) GetPRTitles(prNumbers []int) (map[int]PRInfo, error) {
	if len(prNumbers) == 0 {
		return make(map[int]PRInfo), nil
	}

	// Build GraphQL query with aliases for each PR
	// e.g., query { repository(owner:"o", name:"r") { pr1: pullRequest(number:1) { number title } ... } }
	var queryParts []string
	for _, num := range prNumbers {
		queryParts = append(queryParts, fmt.Sprintf("pr%d: pullRequest(number: %d) { number title }", num, num))
	}

	query := fmt.Sprintf(`query {
		repository(owner: %q, name: %q) {
			%s
		}
	}`, c.owner, c.repo, strings.Join(queryParts, "\n\t\t\t"))

	reqBody := map[string]any{"query": query}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL request: %w", err)
	}

	// We need custom unmarshaling since the PR aliases are dynamic
	var rawResponse map[string]json.RawMessage
	if err := c.rest.Post("graphql", bytes.NewReader(reqBytes), &rawResponse); err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}

	// Check for GraphQL errors (can occur even with HTTP 200)
	if errorsRaw, hasErrors := rawResponse["errors"]; hasErrors {
		var gqlErrors []struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(errorsRaw, &gqlErrors); err == nil && len(gqlErrors) > 0 {
			return nil, fmt.Errorf("GraphQL error: %s", gqlErrors[0].Message)
		}
	}

	// Parse the response
	result := make(map[int]PRInfo)

	dataRaw, ok := rawResponse["data"]
	if !ok {
		return result, nil
	}

	var data struct {
		Repository map[string]*struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("parse GraphQL response: %w", err)
	}

	for _, pr := range data.Repository {
		if pr != nil {
			result[pr.Number] = PRInfo{
				Number: pr.Number,
				Title:  pr.Title,
			}
		}
	}

	return result, nil
}

// Comment represents a GitHub issue/PR comment.
type Comment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

// Client wraps a REST client with repo context.
type Client struct {
	rest  RESTClient
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

// NewClientWithREST creates a client with a custom REST implementation.
// This is primarily useful for testing.
func NewClientWithREST(rest RESTClient, owner, repo string) *Client {
	return &Client{
		rest:  rest,
		owner: owner,
		repo:  repo,
	}
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

// CreateDraftPR creates a new pull request as a draft.
func (c *Client) CreateDraftPR(head, base, title, body string) (int, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls", c.owner, c.repo)

	request := struct {
		Head  string `json:"head"`
		Base  string `json:"base"`
		Title string `json:"title"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}{
		Head:  head,
		Base:  base,
		Title: title,
		Body:  body,
		Draft: true,
	}

	reqBody, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	var response PR
	err = c.rest.Post(path, bytes.NewReader(reqBody), &response)
	if err != nil {
		return 0, fmt.Errorf("failed to create draft PR: %w", err)
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

// MarkPRReady converts a draft PR to ready for review.
// Uses the GraphQL API since REST doesn't support this operation.
func (c *Client) MarkPRReady(prNumber int) error {
	// First get the PR's node_id for GraphQL
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", c.owner, c.repo, prNumber)

	var prData struct {
		NodeID string `json:"node_id"`
	}
	if err := c.rest.Get(path, &prData); err != nil {
		return fmt.Errorf("failed to get PR #%d: %w", prNumber, err)
	}

	// Use GraphQL mutation to mark ready
	query := `mutation($id: ID!) {
		markPullRequestReadyForReview(input: {pullRequestId: $id}) {
			pullRequest { number }
		}
	}`

	variables := map[string]any{"id": prData.NodeID}
	reqBody := map[string]any{
		"query":     query,
		"variables": variables,
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	if err := c.rest.Post("graphql", bytes.NewReader(reqBytes), nil); err != nil {
		return fmt.Errorf("failed to mark PR #%d ready: %w", prNumber, err)
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

// PRURL returns the web URL for a pull request.
func (c *Client) PRURL(number int) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", c.owner, c.repo, number)
}

// RepoURL returns the base URL for the repository (e.g., "https://github.com/owner/repo").
func (c *Client) RepoURL() string {
	return fmt.Sprintf("https://github.com/%s/%s", c.owner, c.repo)
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

// FindPRByHead finds an open PR with the given head branch.
// Returns nil, nil if no PR exists for this branch.
func (c *Client) FindPRByHead(branch string) (*PR, error) {
	// GitHub API expects "owner:branch" format
	path := fmt.Sprintf("repos/%s/%s/pulls?head=%s:%s&state=open",
		c.owner, c.repo, c.owner, branch)

	var prs []PR
	if err := c.rest.Get(path, &prs); err != nil {
		return nil, fmt.Errorf("find PR for branch %q: %w", branch, err)
	}

	if len(prs) == 0 {
		return nil, nil
	}

	return &prs[0], nil
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
