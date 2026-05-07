package commentwarriorruntime

import (
	"strings"
	"testing"
)

func TestDefaultFindingMessageDoesNotCarrySourcePromptInstruction(t *testing.T) {
	t.Parallel()

	if strings.Contains(defaultFindingMessage, "<source_review_prompt>") {
		t.Fatalf("defaultFindingMessage should not mention <source_review_prompt>: %q", defaultFindingMessage)
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
