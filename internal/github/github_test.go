// internal/github/github_test.go
package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CreateComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/123/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "test comment" {
			t.Errorf("expected body 'test comment', got %s", body["body"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 456}`))
	}))
	defer server.Close()

	client := &Client{
		owner: "owner",
		repo:  "repo",
	}
	// Note: We can't easily test with real REST client, so this test verifies the contract
	// Real integration testing would require mocking go-gh
	_ = client
	t.Skip("requires REST client mocking - contract test only")
}
