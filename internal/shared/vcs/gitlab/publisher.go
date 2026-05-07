package gitlab

import (
	"context"
	"log/slog"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
)

// Compile-time check that Publisher implements vcs.Publisher.
var _ vcs.Publisher = (*Publisher)(nil)

// Publisher publishes a FinalReview to a GitLab merge request.
type Publisher struct {
	client        *Client
	clearComments bool
}

// NewPublisher creates a new GitLab publisher.
// When clearComments is true, open unanswered discussions are deleted before posting.
func NewPublisher(client *Client, clearComments bool) *Publisher {
	return &Publisher{client: client, clearComments: clearComments}
}

// Publish posts the review summary as a MR note and individual findings as
// inline diff notes.
func (p *Publisher) Publish(ctx context.Context, review *models.FinalReview, inline []vcs.DiffFinding,
	sourceBranch, targetBranch string) error {
	mr, err := p.client.FindMergeRequest(ctx, sourceBranch, targetBranch)
	if err != nil {
		return err
	}

	if p.clearComments {
		deleted, err := p.client.ClearMRComments(ctx, mr.IID)
		if err != nil {
			slog.Warn("failed to clear MR comments", "error", err)
		} else {
			slog.Info("cleared MR comments", "deleted", deleted)
		}
	}

	refs, err := p.client.GetMRDiffRefs(ctx, mr.IID)
	if err != nil {
		return err
	}
	metadata := vcs.MarkerMetadata{
		BaseSHA:  refs.BaseSHA,
		HeadSHA:  refs.HeadSHA,
		StartSHA: refs.StartSHA,
	}

	summaryNote := vcs.FormatReviewNoteWithMetadata(review, metadata)
	if err := p.client.PostMRNote(ctx, mr.IID, summaryNote); err != nil {
		return err
	}

	if len(inline) == 0 {
		return nil
	}

	// Split findings by InDiff: only lines confirmed inside a hunk can be posted inline.
	var toPost, fallback []vcs.DiffFinding
	for _, df := range inline {
		if df.InDiff {
			toPost = append(toPost, df)
		} else {
			fallback = append(fallback, df)
		}
	}

	// Fallback findings → plain MR comments (no diff refs required).
	var fallbackPosted, fallbackFailed int
	for _, df := range fallback {
		note := vcs.FormatFallbackNoteWithMetadata(df, metadata)
		if err := p.client.PostMRNote(ctx, mr.IID, note); err != nil {
			fallbackFailed++
			slog.Warn("failed to post fallback note", "path", df.NewPath, "line", df.NewLine+df.OldLine, "error", err)
			continue
		}
		fallbackPosted++
	}
	if len(fallback) > 0 {
		slog.Info("fallback notes posted", "posted", fallbackPosted, "failed", fallbackFailed)
	}

	if len(toPost) == 0 {
		return nil
	}

	var posted, failed int
	for _, df := range toPost {
		note := vcs.FormatFindingNoteWithMetadata(df.Source, metadata)
		if err := p.client.PostDiffNote(ctx, mr.IID, note, refs, df.OldPath, df.NewPath, df.OldLine, df.NewLine); err != nil {
			failed++
			slog.Warn("failed to post inline note",
				"old_path", df.OldPath,
				"new_path", df.NewPath,
				"old_line", df.OldLine,
				"new_line", df.NewLine,
				"issue", df.Source.IssueContent,
				"error", err,
			)
			continue
		}
		posted++
	}

	slog.Info("inline notes posted", "posted", posted, "failed", failed, "total", len(toPost))
	return nil
}
