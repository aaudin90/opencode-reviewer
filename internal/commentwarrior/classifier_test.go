package commentwarrior

import (
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

func TestContainsAIMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "plain marker", body: "#ai", want: true},
		{name: "marker in sentence", body: "please check #ai", want: true},
		{name: "uppercase marker", body: "please check #AI", want: true},
		{name: "mention marker is not accepted", body: "@#ai", want: false},
		{name: "embedded prefix is not accepted", body: "x#ai", want: false},
		{name: "embedded suffix is not accepted", body: "#air", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := containsAIMarker(strings.ToLower(tt.body)); got != tt.want {
				t.Fatalf("containsAIMarker(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

func TestShouldProcessDiscussion_AIFindingRequiresResolvedOrHumanNote(t *testing.T) {
	t.Parallel()

	const botID = 10
	findingBody := vcs.AppendMarker("finding", "finding", vcs.MarkerMetadata{File: "file.kt"})

	tests := []struct {
		name string
		d    gitlab.Discussion
		want bool
	}{
		{
			name: "open untouched ai finding is skipped",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true},
			}},
			want: false,
		},
		{
			name: "resolved ai finding is processed",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true, Resolved: true},
			}},
			want: true,
		},
		{
			name: "ai finding with human reply is processed",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true},
				{ID: 2, Body: "please check", Author: gitlab.Author{ID: 20}},
			}},
			want: true,
		},
		{
			name: "resolved ai finding answered by bot without closure marker is processed",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true, Resolved: true},
				{ID: 2, Body: "fixed, thanks", Author: gitlab.Author{ID: botID}},
			}},
			want: true,
		},
		{
			name: "resolved ai finding with closure marker is skipped",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true, Resolved: true},
				{
					ID:     2,
					Body:   vcs.AppendMarker("fixed, thanks", ClosureConfirmedMarkerKind, vcs.MarkerMetadata{}),
					Author: gitlab.Author{ID: botID},
				},
			}},
			want: false,
		},
		{
			name: "open ai finding already answered by bot is skipped",
			d: gitlab.Discussion{Notes: []gitlab.Note{
				{ID: 1, Body: findingBody, Author: gitlab.Author{ID: botID}, Resolvable: true},
				{ID: 2, Body: "please check", Author: gitlab.Author{ID: 20}},
				{ID: 3, Body: "still valid", Author: gitlab.Author{ID: botID}},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			classification := ClassifyDiscussion(tt.d, botID)
			if got := ShouldProcessDiscussion(classification, tt.d, botID); got != tt.want {
				t.Fatalf("ShouldProcessDiscussion() = %v, want %v", got, tt.want)
			}
		})
	}
}
