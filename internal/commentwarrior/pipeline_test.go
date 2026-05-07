package commentwarrior

import (
	"testing"

	commentwarriorruntime "github.com/aaudin90/opencode-reviewer/internal/commentwarrior/runtime"
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
