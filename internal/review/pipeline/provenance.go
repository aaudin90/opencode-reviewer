package pipeline

import "github.com/aaudin90/opencode-reviewer/internal/shared/models"

func applyFindingProvenance(review *models.FinalReview, phase1 []*models.ReviewResult) {
	if review == nil || len(review.Findings) == 0 || len(phase1) == 0 {
		return
	}
	byReviewer := make(map[string][]models.ReviewMessageRef)
	allRefs := make([]models.ReviewMessageRef, 0, len(phase1))
	for _, r := range phase1 {
		if r == nil || r.MessageRef.SHA256 == "" && r.MessageRef.ID == "" && r.MessageRef.Path == "" {
			continue
		}
		allRefs = append(allRefs, r.MessageRef)
		if r.ReviewerName != "" {
			byReviewer[r.ReviewerName] = append(byReviewer[r.ReviewerName], r.MessageRef)
		}
	}

	for i := range review.Findings {
		seen := map[models.ReviewMessageRef]bool{}
		var matched []models.ReviewMessageRef
		ambiguous := false
		for _, source := range review.Findings[i].Sources {
			refs := byReviewer[source]
			if len(refs) != 1 {
				if len(refs) > 1 {
					ambiguous = true
				}
				continue
			}
			if !seen[refs[0]] {
				seen[refs[0]] = true
				matched = append(matched, refs[0])
			}
		}
		if len(matched) > 0 && !ambiguous {
			review.Findings[i].SourceMessageRefs = matched
			continue
		}
		if len(allRefs) > 0 {
			review.Findings[i].FallbackMessageRefs = allRefs
		}
	}
}
