package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(url string) *Client {
	return NewClient(Config{
		URL:       url,
		Token:     "test-token",
		ProjectID: 42,
	})
}

func TestFindMergeRequest_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/merge_requests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{
			{IID: 7, Title: "Feature", State: "opened", SHA: "abc123"},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	mr, err := c.FindMergeRequest(context.Background(), "feature", "main")
	if err != nil {
		t.Fatalf("FindMergeRequest: %v", err)
	}
	if mr.IID != 7 {
		t.Errorf("IID = %d, want 7", mr.IID)
	}
	if mr.Title != "Feature" {
		t.Errorf("Title = %q, want Feature", mr.Title)
	}
}

func TestFindMergeRequest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.FindMergeRequest(context.Background(), "feature", "main")
	if err == nil {
		t.Fatal("expected error for empty MR list")
	}
	if !strings.Contains(err.Error(), "no open merge request") {
		t.Errorf("error = %v, want 'no open merge request'", err)
	}
}

func TestGetMRDiffRefs_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/merge_requests/7/versions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"base_commit_sha":"base1","head_commit_sha":"head1","start_commit_sha":"start1"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	refs, err := c.GetMRDiffRefs(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetMRDiffRefs: %v", err)
	}
	if refs.BaseSHA != "base1" || refs.HeadSHA != "head1" || refs.StartSHA != "start1" {
		t.Errorf("unexpected refs: %+v", refs)
	}
}

func TestPostMRNote_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.PostMRNote(context.Background(), 7, "Test note")
	if err != nil {
		t.Fatalf("PostMRNote: %v", err)
	}
}

func TestPostMRNote_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.PostMRNote(context.Background(), 7, "Test note")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want to contain 403", err)
	}
}

func TestPostDiffNote_NewLinePayload(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 2}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	refs := &DiffRefs{BaseSHA: "b", HeadSHA: "h", StartSHA: "s"}
	err := c.PostDiffNote(context.Background(), 7, "inline comment", refs, "main.go", "main.go", 0, 42)
	if err != nil {
		t.Fatalf("PostDiffNote: %v", err)
	}

	pos, ok := got["position"].(map[string]any)
	if !ok {
		t.Fatal("missing position object")
	}
	if pos["position_type"] != "text" {
		t.Errorf("position_type = %v, want text", pos["position_type"])
	}
	if pos["new_path"] != "main.go" {
		t.Errorf("new_path = %v, want main.go", pos["new_path"])
	}
	if pos["new_line"] != float64(42) {
		t.Errorf("new_line = %v, want 42", pos["new_line"])
	}
	if _, hasOldLine := pos["old_line"]; hasOldLine {
		t.Errorf("old_line should be absent for new-line findings, got %v", pos["old_line"])
	}
	if pos["base_sha"] != "b" || pos["head_sha"] != "h" || pos["start_sha"] != "s" {
		t.Errorf("unexpected SHA refs in position: %+v", pos)
	}
}

func TestPostDiffNote_OldLinePayload(t *testing.T) {
	var got map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 3}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	refs := &DiffRefs{BaseSHA: "b", HeadSHA: "h", StartSHA: "s"}
	err := c.PostDiffNote(context.Background(), 7, "deleted line comment", refs, "main.go", "main.go", 10, 0)
	if err != nil {
		t.Fatalf("PostDiffNote: %v", err)
	}

	pos, ok := got["position"].(map[string]any)
	if !ok {
		t.Fatal("missing position object")
	}
	if pos["old_line"] != float64(10) {
		t.Errorf("old_line = %v, want 10", pos["old_line"])
	}
	if _, hasNewLine := pos["new_line"]; hasNewLine {
		t.Errorf("new_line should be absent for old-line findings, got %v", pos["new_line"])
	}
}

func registerCurrentUser(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CurrentUser{ID: 99})
	})
}

func TestClearMRComments_DeletesOpenSingle(t *testing.T) {
	var deleteCount int

	discussions := []Discussion{
		{
			ID:             "d1",
			IndividualNote: true,
			Notes:          []Note{{ID: 101, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 99}}},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/merge_requests/10/notes/101", func(w http.ResponseWriter, _ *http.Request) {
		deleteCount++
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if deleteCount != 1 {
		t.Errorf("DELETE calls = %d, want 1", deleteCount)
	}
}

func TestClearMRComments_SkipsResolved(t *testing.T) {
	discussions := []Discussion{
		{
			ID:    "d1",
			Notes: []Note{{ID: 102, System: false, Resolvable: true, Resolved: true, Author: Author{ID: 99}}},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/merge_requests/10/notes/102", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("DELETE should not be called for a resolved discussion")
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestClearMRComments_SkipsReplied(t *testing.T) {
	discussions := []Discussion{
		{
			ID: "d1",
			Notes: []Note{
				{ID: 103, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 99}},
				{ID: 104, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 99}},
			},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (discussion has replies)", deleted)
	}
}

func TestClearMRComments_SkipsSystem(t *testing.T) {
	discussions := []Discussion{
		{
			ID:    "d1",
			Notes: []Note{{ID: 105, System: true, Resolvable: false, Resolved: false, Author: Author{ID: 99}}},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (system note)", deleted)
	}
}

func TestClearMRComments_SkipsOtherAuthor(t *testing.T) {
	discussions := []Discussion{
		{
			ID:    "d1",
			Notes: []Note{{ID: 106, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 55}}},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})
	mux.HandleFunc("DELETE /api/v4/projects/42/merge_requests/10/notes/106", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("DELETE should not be called for a note by another author")
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (note by another author)", deleted)
	}
}

func TestClearMRComments_Pagination(t *testing.T) {
	var getCount int
	var deleteCount int

	// Build 100 discussions for page 1 and 1 for page 2.
	page1 := make([]Discussion, 100)
	for i := range page1 {
		page1[i] = Discussion{
			ID:    fmt.Sprintf("d%d", i),
			Notes: []Note{{ID: i + 200, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 99}}},
		}
	}
	page2 := []Discussion{
		{ID: "d100", Notes: []Note{{ID: 300, System: false, Resolvable: false, Resolved: false, Author: Author{ID: 99}}}},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, r *http.Request) {
		getCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	})
	// Handle all DELETE requests under the notes path.
	mux.HandleFunc("/api/v4/projects/42/merge_requests/10/notes/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount++
			w.WriteHeader(http.StatusNoContent)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(srv.URL)
	deleted, err := c.ClearMRComments(context.Background(), 10)
	if err != nil {
		t.Fatalf("ClearMRComments: %v", err)
	}
	if getCount != 2 {
		t.Errorf("GET requests = %d, want 2 (pagination)", getCount)
	}
	if deleted != 101 {
		t.Errorf("deleted = %d, want 101", deleted)
	}
	if deleteCount != 101 {
		t.Errorf("DELETE calls = %d, want 101", deleteCount)
	}
}

func TestPrivateTokenHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("PRIVATE-TOKEN")
		if token != "test-token" {
			t.Errorf("PRIVATE-TOKEN = %q, want test-token", token)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	// Any request — just check header.
	_, _ = c.FindMergeRequest(context.Background(), "a", "b")
}

func TestListDiscussionsPagination(t *testing.T) {
	page1 := make([]Discussion, 100)
	for i := range page1 {
		page1[i] = Discussion{ID: fmt.Sprintf("d%d", i)}
	}
	page2 := []Discussion{{ID: "d100"}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			_ = json.NewEncoder(w).Encode(page1)
			return
		}
		_ = json.NewEncoder(w).Encode(page2)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := newTestClient(srv.URL).ListDiscussions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListDiscussions: %v", err)
	}
	if len(got) != 101 {
		t.Fatalf("len = %d, want 101", len(got))
	}
}

func TestReplyToDiscussionPayload(t *testing.T) {
	var got map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/discussions/d1/notes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	err := newTestClient(srv.URL).ReplyToDiscussion(context.Background(), 10, "d1", "reply body")
	if err != nil {
		t.Fatalf("ReplyToDiscussion: %v", err)
	}
	if got["body"] != "reply body" {
		t.Fatalf("body = %q", got["body"])
	}
}

func TestSetDiscussionResolvedPayload(t *testing.T) {
	var got map[string]bool
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/42/merge_requests/10/discussions/d1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	err := newTestClient(srv.URL).SetDiscussionResolved(context.Background(), 10, "d1", true)
	if err != nil {
		t.Fatalf("SetDiscussionResolved: %v", err)
	}
	if !got["resolved"] {
		t.Fatalf("resolved = false, want true")
	}
}
