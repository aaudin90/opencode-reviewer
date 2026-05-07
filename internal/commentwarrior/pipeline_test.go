package commentwarrior

import (
	"strings"
	"testing"

	commentwarriorruntime "github.com/aaudin90/opencode-reviewer/internal/commentwarrior/runtime"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

func TestMessageForClassification(t *testing.T) {
	t.Parallel()

	resources := &commentwarriorruntime.RuntimeResources{
		FindingMessage: "finding message",
		MentionMessage: "mention message",
	}

	tests := []struct {
		name           string
		classification Classification
		want           string
	}{
		{name: "ai finding", classification: ClassAIFinding, want: "finding message"},
		{name: "human mention", classification: ClassHumanMentionAI, want: "mention message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := messageForClassification(resources, tt.classification); got != tt.want {
				t.Fatalf("messageForClassification() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPromptAddsSourcePromptInstructionOnlyWhenLoaded(t *testing.T) {
	t.Parallel()

	withSource := buildPrompt("message", "source prompt", "task")
	if !strings.Contains(withSource, "<source_review_prompt> block is attached below") {
		t.Fatalf("buildPrompt() missing source prompt instruction: %q", withSource)
	}

	withoutSource := buildPrompt("message", "", "task")
	if strings.Contains(withoutSource, "<source_review_prompt>") {
		t.Fatalf("buildPrompt() added source prompt instruction without source: %q", withoutSource)
	}
}

func TestFilterProcessableDiscussions(t *testing.T) {
	t.Parallel()

	const botID = 10
	findingBody := vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{File: "file.kt"})
	p := NewPipeline(PipelineConfig{MaxDiscussions: 2}, nil, nil, nil)
	discussions := []gitlab.Discussion{
		{
			ID: "ignored",
			Notes: []gitlab.Note{
				{ID: 1, Body: "human note", Author: gitlab.Author{ID: 20}},
			},
		},
		{
			ID: "mention",
			Notes: []gitlab.Note{
				{ID: 2, Body: "#ai please check", Author: gitlab.Author{ID: 20}},
			},
		},
		{
			ID: "finding",
			Notes: []gitlab.Note{
				{ID: 3, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true},
				{ID: 4, Body: "please recheck", Author: gitlab.Author{ID: 20}},
			},
		},
		{
			ID: "limited-out",
			Notes: []gitlab.Note{
				{ID: 5, Body: "#ai also check", Author: gitlab.Author{ID: 20}},
			},
		},
	}

	got := p.filterProcessableDiscussions(discussions, botID)
	if len(got) != 2 {
		t.Fatalf("filterProcessableDiscussions() returned %d discussions, want 2", len(got))
	}
	if got[0].discussion.ID != "mention" || got[0].classification != ClassHumanMentionAI {
		t.Fatalf("first discussion = %#v", got[0])
	}
	if got[1].discussion.ID != "finding" || got[1].classification != ClassAIFinding {
		t.Fatalf("second discussion = %#v", got[1])
	}
}

func TestFilterProcessableDiscussionsDiscussionID(t *testing.T) {
	t.Parallel()

	const botID = 10
	p := NewPipeline(PipelineConfig{DiscussionID: "target"}, nil, nil, nil)
	discussions := []gitlab.Discussion{
		{
			ID: "other",
			Notes: []gitlab.Note{
				{ID: 1, Body: "#ai other", Author: gitlab.Author{ID: 20}},
			},
		},
		{
			ID: "target",
			Notes: []gitlab.Note{
				{ID: 2, Body: "#ai target", Author: gitlab.Author{ID: 20}},
			},
		},
	}

	got := p.filterProcessableDiscussions(discussions, botID)
	if len(got) != 1 {
		t.Fatalf("filterProcessableDiscussions() returned %d discussions, want 1", len(got))
	}
	if got[0].discussion.ID != "target" {
		t.Fatalf("discussion ID = %q, want target", got[0].discussion.ID)
	}
}
