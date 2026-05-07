package commentwarrior

import (
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

type Classification string

const (
	ClassIgnore         Classification = "ignore"
	ClassAIFinding      Classification = "ai_finding"
	ClassHumanMentionAI Classification = "human_mention_ai"
)

const ClosureConfirmedMarkerKind = "closure-confirmed"

func ClassifyDiscussion(d gitlab.Discussion, botUserID int) Classification {
	if len(d.Notes) == 0 {
		return ClassIgnore
	}
	if hasUnhandledAIMention(d, botUserID) {
		return ClassHumanMentionAI
	}
	first := d.Notes[0]
	if first.Author.ID == botUserID {
		for _, marker := range vcs.ParseMarkers(first.Body) {
			if marker.Kind == "finding" {
				return ClassAIFinding
			}
		}
	}
	return ClassIgnore
}

func ShouldProcessDiscussion(classification Classification, d gitlab.Discussion, botUserID int) bool {
	switch classification {
	case ClassHumanMentionAI:
		return hasUnhandledAIMention(d, botUserID)
	case ClassAIFinding:
		if discussionResolved(d) {
			return !LatestBotNoteHasClosureConfirmedMarker(d, botUserID)
		}
		return latestHumanNote(d, botUserID) != nil && !AlreadyHandledAIFinding(d, botUserID)
	default:
		return false
	}
}

func LatestBotNoteHasClosureConfirmedMarker(d gitlab.Discussion, botUserID int) bool {
	for i := len(d.Notes) - 1; i >= 0; i-- {
		n := d.Notes[i]
		if n.System || n.Author.ID != botUserID {
			continue
		}
		for _, marker := range vcs.ParseMarkers(n.Body) {
			if marker.Kind == ClosureConfirmedMarkerKind {
				return true
			}
		}
		return false
	}
	return false
}

func AlreadyHandledAIFinding(d gitlab.Discussion, botUserID int) bool {
	if len(d.Notes) <= 1 {
		return false
	}
	for i := len(d.Notes) - 1; i >= 1; i-- {
		n := d.Notes[i]
		if n.System {
			continue
		}
		return n.Author.ID == botUserID
	}
	return false
}

func discussionResolved(d gitlab.Discussion) bool {
	for _, n := range d.Notes {
		if n.Resolvable && n.Resolved {
			return true
		}
	}
	return false
}

func containsAIMarker(body string) bool {
	for offset := 0; ; {
		idx := strings.Index(body[offset:], "#ai")
		if idx == -1 {
			return false
		}
		idx += offset
		before := idx == 0 || !isMarkerChar(body[idx-1])
		afterIdx := idx + len("#ai")
		after := afterIdx == len(body) || !isMarkerChar(body[afterIdx])
		if before && after {
			return true
		}
		offset = idx + len("#ai")
	}
}

func isMarkerChar(b byte) bool {
	return b == '@' || b == '#' || b == '_' || b == '-' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z')
}

func latestHumanNote(d gitlab.Discussion, botUserID int) *gitlab.Note {
	for i := len(d.Notes) - 1; i >= 0; i-- {
		n := d.Notes[i]
		if n.System || n.Author.ID == botUserID {
			continue
		}
		return &d.Notes[i]
	}
	return nil
}

func hasUnhandledAIMention(d gitlab.Discussion, botUserID int) bool {
	for i := len(d.Notes) - 1; i >= 0; i-- {
		n := d.Notes[i]
		if n.System {
			continue
		}
		if n.Author.ID == botUserID {
			return false
		}
		if containsAIMarker(strings.ToLower(n.Body)) {
			return true
		}
	}
	return false
}
