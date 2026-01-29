// internal/github/github_test.go
package github

import (
	"encoding/json"
	"errors"
	"io"
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
