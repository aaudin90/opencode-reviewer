package commentwarrior

import (
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

func TestBuildTaskIncludesSourceReviewPrompt(t *testing.T) {
	t.Parallel()

	prompt := "Check SQL injection paths carefully.\nPrefer real exploitability."

	task := buildTask(taskConfig{
		SourceReviewPrompt: prompt,
		Discussion:         gitlab.Discussion{Notes: []gitlab.Note{{Body: "finding"}}},
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

	task := buildTask(taskConfig{
		Discussion: gitlab.Discussion{Notes: []gitlab.Note{{Body: "plain note"}}},
	})
	if strings.Contains(task, "## Source Review Prompt") {
		t.Fatalf("unexpected source prompt section:\n%s", task)
	}
	if strings.Contains(task, "<source_review_prompt>") {
		t.Fatalf("unexpected source prompt tag:\n%s", task)
	}
}

func TestBuildTaskIncludesDiffContextPath(t *testing.T) {
	t.Parallel()

	diffPath := "/repo/.opencode-review/diff.md"
	task := buildTask(taskConfig{
		DiffPath:   diffPath,
		Discussion: gitlab.Discussion{Notes: []gitlab.Note{{Body: "finding"}}},
	})

	if !strings.Contains(task, "## MR Diff Context") {
		t.Fatalf("task missing diff context section:\n%s", task)
	}
	if !strings.Contains(task, "`"+diffPath+"`") {
		t.Fatalf("task missing diff path:\n%s", task)
	}
}

func TestBuildTaskCompactsDiscussionAndStripsMarkers(t *testing.T) {
	t.Parallel()

	body := vcs.AppendMarker("finding body", "finding", vcs.MarkerMetadata{
		BaseSHA:   "base",
		HeadSHA:   "head",
		StartSHA:  "start",
		File:      "path.kt",
		StartLine: 9,
	})
	task := buildTask(taskConfig{
		Discussion: gitlab.Discussion{
			ID:             "discussion-id",
			IndividualNote: true,
			Notes: []gitlab.Note{{
				ID:         10,
				Body:       body,
				Resolvable: true,
				Resolved:   false,
				Author:     gitlab.Author{ID: 123, Username: "bot", Name: "Bot"},
				Position: &gitlab.Position{
					PositionType: "text",
					BaseSHA:      "base",
					HeadSHA:      "head",
					StartSHA:     "start",
					NewPath:      "path.kt",
					NewLine:      9,
				},
			}},
		},
	})

	for _, unexpected := range []string{
		"opencode-reviewer:finding",
		"individual_note",
		"base_sha",
		"head_sha",
		"start_sha",
		"position_type",
		`"id": 123`,
	} {
		if strings.Contains(task, unexpected) {
			t.Fatalf("task contains noisy field %q:\n%s", unexpected, task)
		}
	}
	for _, expected := range []string{
		`"id": "discussion-id"`,
		`"body": "finding body"`,
		`"username": "bot"`,
		`"path": "path.kt"`,
		`"line": 9`,
	} {
		if !strings.Contains(task, expected) {
			t.Fatalf("task missing expected field %q:\n%s", expected, task)
		}
	}
}
