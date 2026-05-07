package commentwarriorruntime

import (
	"strings"
	"testing"
)

func TestDefaultMessageMentionsSourceReviewPrompt(t *testing.T) {
	t.Parallel()

	if !strings.Contains(defaultFindingMessage, "<source_review_prompt>") {
		t.Fatalf("defaultFindingMessage must mention <source_review_prompt>: %q", defaultFindingMessage)
	}
	if !strings.Contains(defaultFindingMessage, "prompt") || !strings.Contains(defaultFindingMessage, "original reviewer comment") {
		t.Fatalf("defaultFindingMessage must explain source_review_prompt meaning: %q", defaultFindingMessage)
	}
}

func TestDefaultMentionMessageMentionsHumanAIRequest(t *testing.T) {
	t.Parallel()

	if !strings.Contains(defaultMentionMessage, "#ai") {
		t.Fatalf("defaultMentionMessage must mention #ai: %q", defaultMentionMessage)
	}
	if !strings.Contains(defaultMentionMessage, "direct human request") {
		t.Fatalf("defaultMentionMessage must explain mention semantics: %q", defaultMentionMessage)
	}
}
