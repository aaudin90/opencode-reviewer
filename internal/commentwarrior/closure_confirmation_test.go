package commentwarrior

import (
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

func TestShouldConfirmClosure(t *testing.T) {
	t.Parallel()

	const botID = 10
	findingBody := vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{})

	tests := []struct {
		name           string
		classification Classification
		discussion     gitlab.Discussion
		decision       Decision
		want           bool
	}{
		{
			name:           "open ai finding resolved by warrior confirms closure",
			classification: ClassAIFinding,
			discussion: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true},
				{ID: 2, Body: "close it", Author: gitlab.Author{ID: 20}},
			}},
			decision: Decision{Action: ActionResolve, Body: "closing"},
			want:     true,
		},
		{
			name:           "resolved ai finding reply confirms closure",
			classification: ClassAIFinding,
			discussion: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true, Resolved: true},
			}},
			decision: Decision{Action: ActionReply, Body: "confirmed"},
			want:     true,
		},
		{
			name:           "ai finding unresolve does not confirm closure",
			classification: ClassAIFinding,
			discussion: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true, Resolved: true},
			}},
			decision: Decision{Action: ActionUnresolve, Body: "still valid"},
			want:     false,
		},
		{
			name:           "human mention resolve does not use closure marker",
			classification: ClassHumanMentionAI,
			discussion: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: "#ai close", Author: gitlab.Author{ID: 20}},
			}},
			decision: Decision{Action: ActionResolve, Body: "closing"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldConfirmClosure(tt.classification, tt.discussion, tt.decision); got != tt.want {
				t.Fatalf("shouldConfirmClosure() = %v, want %v", got, tt.want)
			}
		})
	}
}
