package vcs

import (
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

func TestAppendMarkerAndParse(t *testing.T) {
	body := AppendMarker("note body", "finding", MarkerMetadata{
		BaseSHA:   "base",
		HeadSHA:   "head",
		StartSHA:  "start",
		File:      "main.go",
		StartLine: 10,
		SourceMessageRefs: []models.ReviewMessageRef{
			{ID: "bugs", Path: "reviewer/messages/bugs.md", SHA256: "abc"},
		},
	})

	if !strings.Contains(body, "note body") {
		t.Fatalf("body missing original text: %s", body)
	}
	if strings.Contains(body, "full prompt") {
		t.Fatalf("marker leaked prompt content: %s", body)
	}

	markers := ParseMarkers(body)
	if len(markers) != 1 {
		t.Fatalf("markers = %d, want 1", len(markers))
	}
	if markers[0].Kind != "finding" {
		t.Fatalf("kind = %q, want finding", markers[0].Kind)
	}
	if markers[0].Metadata.BaseSHA != "base" || markers[0].Metadata.HeadSHA != "head" || markers[0].Metadata.StartSHA != "start" {
		t.Fatalf("unexpected refs: %+v", markers[0].Metadata)
	}
	if got := markers[0].Metadata.SourceMessageRefs[0].SHA256; got != "abc" {
		t.Fatalf("sha = %q, want abc", got)
	}
}

func TestParseMarkersIgnoresMalformed(t *testing.T) {
	body := "x\n<!-- opencode-reviewer:finding:v1:not-valid-*** -->\ny"
	if markers := ParseMarkers(body); len(markers) != 0 {
		t.Fatalf("markers = %v, want none", markers)
	}
}
