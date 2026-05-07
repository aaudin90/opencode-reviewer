package commentwarrior

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

func TestBuildTaskIncludesSourceReviewPrompt(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	promptPath := filepath.Join(projectDir, "reviewer", "messages", "security.md")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	prompt := "Check SQL injection paths carefully.\nPrefer real exploitability."
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	task := BuildTask(TaskConfig{
		ProjectDir: projectDir,
		Discussion: gitlab.Discussion{
			Notes: []gitlab.Note{{
				Body: vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{
					SourceMessageRefs: []models.ReviewMessageRef{
						{ID: "inline-1"},
						{ID: "security", Path: "reviewer/messages/security.md"},
					},
				}),
			}},
		},
	})

	if !strings.Contains(task, "## Source Review Prompt") {
		t.Fatalf("task missing source prompt section:\n%s", task)
	}
	if !strings.Contains(task, "<source_review_prompt>\n"+prompt+"\n</source_review_prompt>") {
		t.Fatalf("task missing wrapped source prompt:\n%s", task)
	}
}

func TestBuildTaskSkipsSourceReviewPromptWhenUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		discussion gitlab.Discussion
	}{
		{
			name: "empty ref path",
			discussion: gitlab.Discussion{
				Notes: []gitlab.Note{{
					Body: vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{
						SourceMessageRefs: []models.ReviewMessageRef{{ID: "x"}},
					}),
				}},
			},
		},
		{
			name: "missing file",
			discussion: gitlab.Discussion{
				Notes: []gitlab.Note{{
					Body: vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{
						SourceMessageRefs: []models.ReviewMessageRef{{ID: "x", Path: "missing.md"}},
					}),
				}},
			},
		},
		{
			name: "no finding marker",
			discussion: gitlab.Discussion{
				Notes: []gitlab.Note{{Body: "plain note"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			task := BuildTask(TaskConfig{
				ProjectDir: t.TempDir(),
				Discussion: tt.discussion,
			})
			if strings.Contains(task, "## Source Review Prompt") {
				t.Fatalf("unexpected source prompt section:\n%s", task)
			}
			if strings.Contains(task, "<source_review_prompt>") {
				t.Fatalf("unexpected source prompt tag:\n%s", task)
			}
		})
	}
}
