package vcs

import (
	"context"

	"github.com/aaudin90/opencode-reviewer/internal/models"
)

// Publisher publishes a FinalReview to a VCS platform (e.g. GitLab MR comments).
type Publisher interface {
	Publish(ctx context.Context, review *models.FinalReview, inline []DiffFinding,
		sourceBranch, targetBranch string) error
}
