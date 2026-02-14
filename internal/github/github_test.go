// internal/github/github_test.go
package github

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// mockREST implements RESTClient for testing.
type mockREST struct {
	getFn   func(path string, response any) error
	postFn  func(path string, body io.Reader, response any) error
	patchFn func(path string, body io.Reader, response any) error
}

// Ensure mockREST implements RESTClient interface
// (this is a common idiom)
var _ RESTClient = (*mockREST)(nil)

func (m *mockREST) Get(path string, response any) error {
	if m.getFn != nil {
		return m.getFn(path, response)
	}
	return nil
}

func (m *mockREST) Post(path string, body io.Reader, response any) error {
	if m.postFn != nil {
		return m.postFn(path, body, response)
	}
	return nil
}

func (m *mockREST) Patch(path string, body io.Reader, response any) error {
	if m.patchFn != nil {
		return m.patchFn(path, body, response)
	}
	return nil
}

func TestClient_CreateComment(t *testing.T) {
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			expectedPath := "repos/owner/repo/issues/123/comments"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			// Verify request body
			var req struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if req.Body != "test comment" {
				t.Errorf("expected body %q, got %q", "test comment", req.Body)
			}

			// Populate response
			if c, ok := response.(*Comment); ok {
				c.ID = 456
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	id, err := client.CreateComment(123, "test comment")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 456 {
		t.Errorf("expected id 456, got %d", id)
	}
}

func TestClient_CreateComment_Error(t *testing.T) {
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			return errors.New("API error")
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	_, err := client.CreateComment(123, "test")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_ListComments(t *testing.T) {
	mock := &mockREST{
		getFn: func(path string, response any) error {
			expectedPath := "repos/owner/repo/issues/42/comments"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			// Populate response
			if comments, ok := response.(*[]Comment); ok {
				*comments = []Comment{
					{ID: 1, Body: "first"},
					{ID: 2, Body: "second"},
				}
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	comments, err := client.ListComments(42)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Body != "first" {
		t.Errorf("expected first comment body %q, got %q", "first", comments[0].Body)
	}
}

func TestClient_UpdateComment(t *testing.T) {
	mock := &mockREST{
		patchFn: func(path string, body io.Reader, response any) error {
			expectedPath := "repos/owner/repo/issues/comments/789"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			var req struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if req.Body != "updated content" {
				t.Errorf("expected body %q, got %q", "updated content", req.Body)
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	err := client.UpdateComment(789, "updated content")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetPR(t *testing.T) {
	mock := &mockREST{
		getFn: func(path string, response any) error {
			expectedPath := "repos/owner/repo/pulls/99"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			if pr, ok := response.(*PR); ok {
				pr.Number = 99
				pr.State = "open"
				pr.Draft = true
				pr.Base.Ref = "main"
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	pr, err := client.GetPR(99)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 99 {
		t.Errorf("expected PR number 99, got %d", pr.Number)
	}
	if !pr.Draft {
		t.Error("expected draft PR")
	}
	if pr.Base.Ref != "main" {
		t.Errorf("expected base ref %q, got %q", "main", pr.Base.Ref)
	}
}

func TestClient_CreatePR(t *testing.T) {
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			expectedPath := "repos/owner/repo/pulls"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			var req struct {
				Head  string `json:"head"`
				Base  string `json:"base"`
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if err := json.NewDecoder(body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if req.Head != "feature" {
				t.Errorf("expected head %q, got %q", "feature", req.Head)
			}
			if req.Base != "main" {
				t.Errorf("expected base %q, got %q", "main", req.Base)
			}

			if pr, ok := response.(*PR); ok {
				pr.Number = 101
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	num, err := client.CreatePR("feature", "main", "Add feature", "Description")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 101 {
		t.Errorf("expected PR number 101, got %d", num)
	}
}

func TestClient_CreateDraftPR(t *testing.T) {
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			var req struct {
				Head  string `json:"head"`
				Base  string `json:"base"`
				Title string `json:"title"`
				Body  string `json:"body"`
				Draft bool   `json:"draft"`
			}
			if err := json.NewDecoder(body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if !req.Draft {
				t.Error("expected draft=true")
			}

			if pr, ok := response.(*PR); ok {
				pr.Number = 102
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	num, err := client.CreateDraftPR("feature", "main", "WIP: Add feature", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if num != 102 {
		t.Errorf("expected PR number 102, got %d", num)
	}
}

func TestClient_UpdatePRBase(t *testing.T) {
	mock := &mockREST{
		patchFn: func(path string, body io.Reader, response any) error {
			expectedPath := "repos/owner/repo/pulls/55"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			var req struct {
				Base string `json:"base"`
			}
			if err := json.NewDecoder(body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}
			if req.Base != "develop" {
				t.Errorf("expected base %q, got %q", "develop", req.Base)
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	err := client.UpdatePRBase(55, "develop")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetPRTitles(t *testing.T) {
	t.Run("success with multiple PRs", func(t *testing.T) {
		mock := &mockREST{
			postFn: func(path string, body io.Reader, response any) error {
				if path != "graphql" {
					t.Errorf("expected path %q, got %q", "graphql", path)
				}

				// Return mock GraphQL response
				if raw, ok := response.(*map[string]json.RawMessage); ok {
					*raw = map[string]json.RawMessage{
						"data": json.RawMessage(`{
							"repository": {
								"pr1": {"number": 1, "title": "First PR"},
								"pr2": {"number": 2, "title": "Second PR"}
							}
						}`),
					}
				}
				return nil
			},
		}

		client := NewClientWithREST(mock, "owner", "repo")
		result, err := client.GetPRTitles([]int{1, 2})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d", len(result))
		}
		if result[1].Title != "First PR" {
			t.Errorf("expected title %q, got %q", "First PR", result[1].Title)
		}
		if result[2].Title != "Second PR" {
			t.Errorf("expected title %q, got %q", "Second PR", result[2].Title)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		mock := &mockREST{
			postFn: func(path string, body io.Reader, response any) error {
				t.Error("should not make API call for empty input")
				return nil
			},
		}

		client := NewClientWithREST(mock, "owner", "repo")
		result, err := client.GetPRTitles([]int{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d items", len(result))
		}
	})

	t.Run("GraphQL error in response", func(t *testing.T) {
		mock := &mockREST{
			postFn: func(path string, body io.Reader, response any) error {
				if raw, ok := response.(*map[string]json.RawMessage); ok {
					*raw = map[string]json.RawMessage{
						"errors": json.RawMessage(`[{"message": "Repository not found"}]`),
					}
				}
				return nil
			},
		}

		client := NewClientWithREST(mock, "owner", "repo")
		_, err := client.GetPRTitles([]int{1})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Repository not found") {
			t.Errorf("expected error to contain 'Repository not found', got %q", err.Error())
		}
	})

	t.Run("API error", func(t *testing.T) {
		mock := &mockREST{
			postFn: func(path string, body io.Reader, response any) error {
				return errors.New("network error")
			},
		}

		client := NewClientWithREST(mock, "owner", "repo")
		_, err := client.GetPRTitles([]int{1})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("some PRs not found", func(t *testing.T) {
		mock := &mockREST{
			postFn: func(path string, body io.Reader, response any) error {
				// PR 2 doesn't exist (null in response)
				if raw, ok := response.(*map[string]json.RawMessage); ok {
					*raw = map[string]json.RawMessage{
						"data": json.RawMessage(`{
							"repository": {
								"pr1": {"number": 1, "title": "First PR"},
								"pr2": null
							}
						}`),
					}
				}
				return nil
			},
		}

		client := NewClientWithREST(mock, "owner", "repo")
		result, err := client.GetPRTitles([]int{1, 2})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1 result (PR 2 not found), got %d", len(result))
		}
		if _, ok := result[1]; !ok {
			t.Error("expected PR 1 to be in result")
		}
	})
}

func TestClient_FindPRByHead(t *testing.T) {
	mock := &mockREST{
		getFn: func(path string, response any) error {
			expectedPath := "repos/owner/repo/pulls?head=owner:feature-branch&state=open"
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
			}

			if prs, ok := response.(*[]PR); ok {
				*prs = []PR{
					{Number: 42, State: "open", Draft: false},
				}
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	pr, err := client.FindPRByHead("feature-branch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr == nil {
		t.Fatal("expected PR, got nil")
		return
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
}

func TestClient_FindPRByHead_NotFound(t *testing.T) {
	mock := &mockREST{
		getFn: func(path string, response any) error {
			// Return empty slice (no PRs)
			if prs, ok := response.(*[]PR); ok {
				*prs = []PR{}
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	pr, err := client.FindPRByHead("nonexistent-branch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Errorf("expected nil PR, got %+v", pr)
	}
}

func TestClient_FindPRByHead_Error(t *testing.T) {
	mock := &mockREST{
		getFn: func(path string, response any) error {
			return errors.New("API error")
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	_, err := client.FindPRByHead("feature")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_CreateSubmitPR(t *testing.T) {
	var capturedBody map[string]any
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			if path != "repos/owner/repo/pulls" {
				t.Errorf("expected path %q, got %q", "repos/owner/repo/pulls", path)
			}

			if err := json.NewDecoder(body).Decode(&capturedBody); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}

			if pr, ok := response.(*PR); ok {
				pr.Number = 42
				pr.Title = capturedBody["title"].(string)
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	pr, err := client.CreateSubmitPR("feature-branch", "main", "Feature Branch", "PR body from commits", false)

	if err != nil {
		t.Fatalf("CreateSubmitPR failed: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
	if capturedBody["draft"] != false {
		t.Errorf("expected draft=false, got %v", capturedBody["draft"])
	}
	// Title should be passed through
	if capturedBody["title"] != "Feature Branch" {
		t.Errorf("expected title='Feature Branch', got %v", capturedBody["title"])
	}
	// Body should be passed through
	if capturedBody["body"] != "PR body from commits" {
		t.Errorf("expected body='PR body from commits', got %v", capturedBody["body"])
	}
}

func TestClient_CreateSubmitPR_Draft(t *testing.T) {
	var capturedBody map[string]any
	mock := &mockREST{
		postFn: func(path string, body io.Reader, response any) error {
			if err := json.NewDecoder(body).Decode(&capturedBody); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}

			if pr, ok := response.(*PR); ok {
				pr.Number = 43
				pr.Draft = true
			}
			return nil
		},
	}

	client := NewClientWithREST(mock, "owner", "repo")
	pr, err := client.CreateSubmitPR("wip-feature", "develop", "WIP Feature", "", true)

	if err != nil {
		t.Fatalf("CreateSubmitPR failed: %v", err)
	}
	if pr.Number != 43 {
		t.Errorf("expected PR number 43, got %d", pr.Number)
	}
	if capturedBody["draft"] != true {
		t.Errorf("expected draft=true, got %v", capturedBody["draft"])
	}
}
