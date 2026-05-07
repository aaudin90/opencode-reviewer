package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
)

// Prepare fetches the diff between branch and baseBranch, parses it,
// filters noise files, and sorts the result.
func Prepare(gitClient *git.Client, branch, baseBranch string) (*Result, error) {
	rawDiff, err := gitClient.DiffForReview(baseBranch, branch)
	if err != nil {
		return nil, fmt.Errorf("diff prepare: %w", err)
	}

	diffStat, err := gitClient.DiffStat(baseBranch, branch)
	if err != nil {
		return nil, fmt.Errorf("diff stat: %w", err)
	}

	commitLog, err := gitClient.Log(baseBranch, branch)
	if err != nil {
		return nil, fmt.Errorf("diff log: %w", err)
	}

	allFiles := parseFiles(rawDiff)

	var files []FileDiff
	var filtered []string

	for _, f := range allFiles {
		if isNoise(f.Path) {
			filtered = append(filtered, f.Path)
			continue
		}
		files = append(files, f)
	}

	sortFiles(files)

	var totalAdded, totalDeleted int
	for _, f := range files {
		totalAdded += f.Added
		totalDeleted += f.Deleted
	}

	return &Result{
		Files:         files,
		FilteredFiles: filtered,
		TotalAdded:    totalAdded,
		TotalDeleted:  totalDeleted,
		DiffStat:      strings.TrimSpace(diffStat),
		CommitLog:     strings.TrimSpace(commitLog),
		Branch:        branch,
		BaseBranch:    baseBranch,
	}, nil
}

// WriteContextFile writes the diff result as a markdown+XML file
// into <projectDir>/.opencode-review/diff.md and returns the file path.
func WriteContextFile(result *Result, projectDir string) (string, error) {
	dir := filepath.Join(projectDir, config.ReviewDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create review dir: %w", err)
	}

	path := filepath.Join(dir, "diff.md")

	var b strings.Builder

	fmt.Fprintf(&b, "# Code Review: %s → %s\n\n", result.Branch, result.BaseBranch)

	// Summary
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- **Branch:** %s\n", result.Branch)
	fmt.Fprintf(&b, "- **Base:** %s\n", result.BaseBranch)
	fmt.Fprintf(&b, "- **Files changed:** %d\n", len(result.Files))
	fmt.Fprintf(&b, "- **Lines added:** %d\n", result.TotalAdded)
	fmt.Fprintf(&b, "- **Lines deleted:** %d\n", result.TotalDeleted)
	fmt.Fprintf(&b, "- **Estimated tokens:** %d\n", EstimateTokens(result))
	b.WriteString("\n")

	// Commit log
	if result.CommitLog != "" {
		b.WriteString("## Commits\n\n")
		b.WriteString("```\n")
		b.WriteString(result.CommitLog)
		b.WriteString("\n```\n\n")
	}

	// File list
	b.WriteString("## Changed Files\n\n")
	for _, f := range result.Files {
		icon := changeTypeIcon(f.ChangeType)
		lang := ""
		if f.Language != "" {
			lang = " [" + f.Language + "]"
		}
		fmt.Fprintf(&b, "- %s `%s`%s (+%d/-%d)\n", icon, f.Path, lang, f.Added, f.Deleted)
	}
	b.WriteString("\n")

	// Filtered files
	if len(result.FilteredFiles) > 0 {
		b.WriteString("## Filtered Files (noise)\n\n")
		for _, f := range result.FilteredFiles {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		b.WriteString("\n")
	}

	// File diffs as XML
	b.WriteString("## Diffs\n\n")
	for _, f := range result.Files {
		if f.OldPath != "" && f.OldPath != f.Path {
			fmt.Fprintf(&b, "<file path=%q old_path=%q language=%q change_type=%q>\n",
				f.Path, f.OldPath, f.Language, f.ChangeType)
		} else {
			fmt.Fprintf(&b, "<file path=%q language=%q change_type=%q>\n",
				f.Path, f.Language, f.ChangeType)
		}
		b.WriteString(stripDiffHeaders(f.Diff))
		if !strings.HasSuffix(f.Diff, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("</file>\n\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("write context file: %w", err)
	}

	return path, nil
}

// EstimateTokens provides a rough token estimate for the diff content.
func EstimateTokens(result *Result) int {
	total := 0
	for _, f := range result.Files {
		total += len(stripDiffHeaders(f.Diff))
	}

	return total / 4
}
