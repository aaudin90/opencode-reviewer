package vcs

import (
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

// FormatReviewNote formats a FinalReview as a Markdown note suitable for
// posting as a VCS merge request comment.
func FormatReviewNote(rev *models.FinalReview) string {
	return formatReviewNote(rev)
}

func FormatReviewNoteWithMetadata(rev *models.FinalReview, metadata MarkerMetadata) string {
	return AppendMarker(formatReviewNote(rev), "summary", metadata)
}

func formatReviewNote(rev *models.FinalReview) string {
	if rev == nil {
		return "## Automated Code Review\n\nReview completed but produced no structured output."
	}

	verdict, emoji := mapVerdict(rev.Verdict)
	if verdict == "" && rev.Raw != "" {
		var b strings.Builder
		b.WriteString("## Automated Code Review\n\n")
		b.WriteString(rev.Raw)
		return b.String()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## %s Automated Code Review — %s\n\n", emoji, verdict)

	if rev.Summary != "" {
		b.WriteString(rev.Summary)
		b.WriteString("\n\n")
	}

	if len(rev.Findings) == 0 {
		b.WriteString("No issues found.\n")
		return b.String()
	}

	b.WriteString("### Findings\n\n")
	for i, f := range rev.Findings {
		fmt.Fprintf(&b, "%d. ", i+1)
		writeFindingSummary(&b, f)
		b.WriteByte('\n')
	}

	return b.String()
}

// FormatFindingNote formats a single FinalFinding as a Markdown note
// suitable for posting as an inline VCS comment.
func FormatFindingNote(f models.FinalFinding) string {
	return formatFindingNote(f)
}

func FormatFindingNoteWithMetadata(f models.FinalFinding, metadata MarkerMetadata) string {
	metadata.File = f.File
	metadata.StartLine = f.StartLine
	metadata.EndLine = f.EndLine
	metadata.SourceMessageRefs = f.SourceMessageRefs
	metadata.FallbackMessageRefs = f.FallbackMessageRefs
	return AppendMarker(formatFindingNote(f), "finding", metadata)
}

func formatFindingNote(f models.FinalFinding) string {
	var b strings.Builder

	badge := confidenceBadge(f.Confidence)
	if len(f.Sources) > 0 {
		fmt.Fprintf(&b, "**%s** (%s)\n\n", badge, strings.Join(f.Sources, ", "))
	} else {
		fmt.Fprintf(&b, "**%s**\n\n", badge)
	}

	if f.IssueContent != "" {
		b.WriteString(f.IssueContent)
		b.WriteString("\n\n")
	}

	if f.Recommendation != "" {
		b.WriteString("**Recommendation:** ")
		b.WriteString(f.Recommendation)
		b.WriteByte('\n')
	}

	return b.String()
}

// FormatFallbackNote formats a DiffFinding as a general MR comment when
// inline posting is not possible (line is outside a diff hunk).
func FormatFallbackNote(df DiffFinding) string {
	return formatFallbackNote(df)
}

func FormatFallbackNoteWithMetadata(df DiffFinding, metadata MarkerMetadata) string {
	metadata.File = df.Source.File
	metadata.StartLine = df.Source.StartLine
	metadata.EndLine = df.Source.EndLine
	metadata.OldPath = df.OldPath
	metadata.NewPath = df.NewPath
	metadata.OldLine = df.OldLine
	metadata.NewLine = df.NewLine
	metadata.SourceMessageRefs = df.Source.SourceMessageRefs
	metadata.FallbackMessageRefs = df.Source.FallbackMessageRefs
	return AppendMarker(formatFallbackNote(df), "finding", metadata)
}

func formatFallbackNote(df DiffFinding) string {
	var b strings.Builder
	line := df.NewLine
	if line == 0 {
		line = df.OldLine
	}
	fmt.Fprintf(&b, "> `%s:%d` *(line outside diff hunk)*\n\n", df.NewPath, line)
	b.WriteString(FormatFindingNote(df.Source))
	return b.String()
}

func writeFindingSummary(b *strings.Builder, f models.FinalFinding) {
	badge := confidenceBadge(f.Confidence)
	loc := formatLocation(f.File, f.StartLine, f.EndLine)

	fmt.Fprintf(b, "**%s** %s", badge, loc)
	if f.IssueContent != "" {
		b.WriteString(" — ")
		b.WriteString(f.IssueContent)
	}

	if f.ExistingCode != "" {
		b.WriteString("\n\n   <details><summary>Code</summary>\n\n   ```\n   ")
		b.WriteString(strings.ReplaceAll(f.ExistingCode, "\n", "\n   "))
		b.WriteString("\n   ```\n\n   </details>")
	}
}

func formatLocation(file string, start, end int) string {
	if file == "" {
		return ""
	}
	if end != start && end > 0 {
		return fmt.Sprintf("`%s:%d\u2013%d`", file, start, end)
	}
	return fmt.Sprintf("`%s:%d`", file, start)
}

func mapVerdict(v string) (label, emoji string) {
	switch strings.ToLower(v) {
	case "approve":
		return "Approved", "\u2705"
	case "request_changes":
		return "Changes Requested", "\U0001f534"
	case "comment_only":
		return "Comments Only", "\U0001f4ac"
	case "skipped":
		return "Skipped", "\u23ed\ufe0f"
	default:
		return "", ""
	}
}

func confidenceBadge(c string) string {
	switch strings.ToLower(c) {
	case "high":
		return "\U0001f534 High"
	case "medium":
		return "\U0001f7e1 Medium"
	case "low":
		return "\U0001f7e2 Low"
	default:
		return c
	}
}
