package promptref

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReviewMessagesWithRefBaseUsesRelativePath(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".opencodereview")
	messagePath := filepath.Join(configDir, "reviewer", "messages", "review_app.md")
	if err := os.MkdirAll(filepath.Dir(messagePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(messagePath, []byte("review app"), 0o600); err != nil {
		t.Fatal(err)
	}

	messages, err := LoadReviewMessagesWithRefBase(configDir, projectDir, []string{messagePath}, nil)
	if err != nil {
		t.Fatalf("LoadReviewMessagesWithRefBase: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}

	want := ".opencodereview/reviewer/messages/review_app.md"
	if got := messages[0].Ref.Path; got != want {
		t.Fatalf("ref path = %q, want %q", got, want)
	}
}
