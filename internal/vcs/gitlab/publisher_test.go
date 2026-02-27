package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/models"
	"github.com/aaudin90/opencode-reviewer/internal/vcs"
)

func TestPublish_FullFlow(t *testing.T) {
	var noteCount atomic.Int32
	var diffNoteCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{
			{IID: 10, Title: "Test MR", State: "opened"},
		})
	})

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"base_commit_sha":"b","head_commit_sha":"h","start_commit_sha":"s"}]`))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		noteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		diffNoteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	review := &models.FinalReview{
		Verdict: "request_changes",
		Summary: "Issues found.",
		Findings: []models.FinalFinding{
			{File: "main.go", StartLine: 10, Confidence: "high", IssueContent: "Bug"},
			{File: "util.go", StartLine: 5, Confidence: "low", IssueContent: "Style"},
		},
	}
	inline := []vcs.DiffFinding{
		{OldPath: "main.go", NewPath: "main.go", NewLine: 10, InDiff: true, Source: review.Findings[0]},
		{OldPath: "util.go", NewPath: "util.go", NewLine: 5, InDiff: true, Source: review.Findings[1]},
	}

	err := pub.Publish(context.Background(), review, inline, "feature", "main")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if noteCount.Load() != 1 {
		t.Errorf("summary notes = %d, want 1", noteCount.Load())
	}
	if diffNoteCount.Load() != 2 {
		t.Errorf("diff notes = %d, want 2", diffNoteCount.Load())
	}
}

func TestPublish_MRNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	err := pub.Publish(context.Background(), &models.FinalReview{Verdict: "approve"}, nil, "feature", "main")
	if err == nil {
		t.Fatal("expected error when MR not found")
	}
	if !strings.Contains(err.Error(), "no open merge request") {
		t.Errorf("error = %v, want 'no open merge request'", err)
	}
}

func TestPublish_NoFindings(t *testing.T) {
	var noteCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		noteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	// Versions endpoint should NOT be called.
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/versions", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("versions endpoint should not be called when there are no findings")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	err := pub.Publish(context.Background(), &models.FinalReview{Verdict: "approve"}, nil, "feature", "main")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if noteCount.Load() != 1 {
		t.Errorf("notes = %d, want 1 (summary only)", noteCount.Load())
	}
}

func TestPublish_DiffNoteFails(t *testing.T) {
	var diffNoteCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"base_commit_sha":"b","head_commit_sha":"h","start_commit_sha":"s"}]`))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		count := diffNoteCount.Add(1)
		if count == 1 {
			// First diff note fails.
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"line not in diff"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	review := &models.FinalReview{
		Verdict: "request_changes",
		Findings: []models.FinalFinding{
			{File: "a.go", StartLine: 1, IssueContent: "fail"},
			{File: "b.go", StartLine: 2, IssueContent: "pass"},
		},
	}
	inline := []vcs.DiffFinding{
		{OldPath: "a.go", NewPath: "a.go", NewLine: 1, InDiff: true, Source: review.Findings[0]},
		{OldPath: "b.go", NewPath: "b.go", NewLine: 2, InDiff: true, Source: review.Findings[1]},
	}

	err := pub.Publish(context.Background(), review, inline, "feature", "main")
	if err != nil {
		t.Fatalf("Publish should not fail when individual diff notes fail: %v", err)
	}
	if diffNoteCount.Load() != 2 {
		t.Errorf("diff note attempts = %d, want 2", diffNoteCount.Load())
	}
}

func TestPublish_FallbackNoteForNotInDiff(t *testing.T) {
	var noteCount atomic.Int32
	var diffNoteCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	// versions should NOT be called — no InDiff findings.
	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/versions", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("versions endpoint should not be called for fallback-only findings")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		noteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		diffNoteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	review := &models.FinalReview{
		Verdict: "request_changes",
		Findings: []models.FinalFinding{
			{File: "renamed.go", StartLine: 5, IssueContent: "issue in renamed file"},
		},
	}
	inline := []vcs.DiffFinding{
		{OldPath: "old/renamed.go", NewPath: "renamed.go", NewLine: 5, InDiff: false, Source: review.Findings[0]},
	}

	err := pub.Publish(context.Background(), review, inline, "feature", "main")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if noteCount.Load() != 2 {
		t.Errorf("notes = %d, want 2 (1 summary + 1 fallback)", noteCount.Load())
	}
	if diffNoteCount.Load() != 0 {
		t.Errorf("discussions = %d, want 0", diffNoteCount.Load())
	}
}

func TestPublish_ClearComments(t *testing.T) {
	var deleteCount atomic.Int32
	var postNoteCount atomic.Int32
	// Track call order: "delete" before "post_note".
	var callOrder []string
	var mu sync.Mutex // protect callOrder

	discussions := []mrDiscussion{
		{
			ID:             "d1",
			IndividualNote: false,
			Notes:          []discussionNote{{ID: 999, System: false, Resolvable: false, Resolved: false, Author: noteAuthor{ID: 99}}},
		},
	}

	mux := http.NewServeMux()
	registerCurrentUser(mux)

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discussions)
	})

	mux.HandleFunc("DELETE /api/v4/projects/42/merge_requests/10/notes/999", func(w http.ResponseWriter, _ *http.Request) {
		deleteCount.Add(1)
		mu.Lock()
		callOrder = append(callOrder, "delete")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		postNoteCount.Add(1)
		mu.Lock()
		callOrder = append(callOrder, "post_note")
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, true)

	err := pub.Publish(context.Background(), &models.FinalReview{Verdict: "approve"}, nil, "feature", "main")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if deleteCount.Load() != 1 {
		t.Errorf("DELETE calls = %d, want 1", deleteCount.Load())
	}
	if postNoteCount.Load() != 1 {
		t.Errorf("POST note calls = %d, want 1", postNoteCount.Load())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(callOrder) < 2 || callOrder[0] != "delete" || callOrder[1] != "post_note" {
		t.Errorf("call order = %v, want [delete post_note]", callOrder)
	}
}

func TestPublish_ClearComments_Disabled(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/discussions", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("discussions endpoint should not be called when clearComments=false")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	err := pub.Publish(context.Background(), &models.FinalReview{Verdict: "approve"}, nil, "feature", "main")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func TestPublish_DiffRefsFails(t *testing.T) {
	var noteCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MergeRequestInfo{{IID: 10}})
	})

	mux.HandleFunc("GET /api/v4/projects/42/merge_requests/10/versions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	mux.HandleFunc("POST /api/v4/projects/42/merge_requests/10/notes", func(w http.ResponseWriter, _ *http.Request) {
		noteCount.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, Token: "tok", ProjectID: 42})
	pub := NewPublisher(client, false)

	review := &models.FinalReview{
		Verdict: "request_changes",
		Findings: []models.FinalFinding{
			{File: "a.go", StartLine: 1, IssueContent: "issue"},
		},
	}
	inline := []vcs.DiffFinding{
		{OldPath: "a.go", NewPath: "a.go", NewLine: 1, InDiff: true, Source: review.Findings[0]},
	}

	err := pub.Publish(context.Background(), review, inline, "feature", "main")
	if err != nil {
		t.Fatalf("Publish should succeed even when diff refs fail: %v", err)
	}
	if noteCount.Load() != 1 {
		t.Errorf("notes = %d, want 1 (summary only, no diff notes)", noteCount.Load())
	}
}
