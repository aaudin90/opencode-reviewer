package commentwarrior

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

type taskConfig struct {
	Discussion         gitlab.Discussion
	ProjectDir         string
	DiffPath           string
	SourceReviewPrompt string
}

func buildTask(cfg taskConfig) string {
	var b strings.Builder
	if data, err := json.MarshalIndent(compactDiscussion(cfg.Discussion), "", "  "); err == nil {
		b.WriteString("## Discussion\n\n```json\n")
		b.Write(data)
		b.WriteString("\n```\n")
	}
	if cfg.DiffPath != "" {
		b.WriteString("\n## MR Diff Context\n\n")
		fmt.Fprintf(&b, "Open and read `%s` when you need the full merge request diff.\n", cfg.DiffPath)
	}
	if prompt := cfg.SourceReviewPrompt; prompt != "" {
		b.WriteString("\n## Source Review Prompt\n\n")
		b.WriteString("<source_review_prompt>\n")
		b.WriteString(prompt)
		if !strings.HasSuffix(prompt, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("</source_review_prompt>\n")
	}
	if ctx := codeContext(cfg.ProjectDir, cfg.Discussion); ctx != "" {
		b.WriteString("\n## Current Code Context\n\n")
		b.WriteString(ctx)
	}
	return b.String()
}

type discussionContext struct {
	ID    string        `json:"id"`
	Notes []noteContext `json:"notes"`
}

type noteContext struct {
	ID         int              `json:"id"`
	Body       string           `json:"body"`
	System     bool             `json:"system,omitempty"`
	Resolvable bool             `json:"resolvable,omitempty"`
	Resolved   bool             `json:"resolved,omitempty"`
	Author     authorContext    `json:"author"`
	Position   *positionContext `json:"position,omitempty"`
}

type authorContext struct {
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
}

type positionContext struct {
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

func compactDiscussion(d gitlab.Discussion) discussionContext {
	notes := make([]noteContext, 0, len(d.Notes))
	for _, n := range d.Notes {
		notes = append(notes, noteContext{
			ID:         n.ID,
			Body:       strings.TrimSpace(vcs.StripMarkers(n.Body)),
			System:     n.System,
			Resolvable: n.Resolvable,
			Resolved:   n.Resolved,
			Author: authorContext{
				Username: n.Author.Username,
				Name:     n.Author.Name,
			},
			Position: compactPosition(n.Position),
		})
	}
	return discussionContext{ID: d.ID, Notes: notes}
}

func compactPosition(p *gitlab.Position) *positionContext {
	if p == nil {
		return nil
	}
	path := p.NewPath
	line := p.NewLine
	if path == "" {
		path = p.OldPath
		line = p.OldLine
	}
	return &positionContext{Path: path, Line: line}
}

func sourceReviewPrompt(projectDir string, d gitlab.Discussion) string {
	path := sourceReviewPromptPath(d)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(path))) // #nosec G304 -- marker path in checked-out project
	if err != nil {
		return ""
	}
	return string(data)
}

func sourceReviewPromptPath(d gitlab.Discussion) string {
	if len(d.Notes) == 0 {
		return ""
	}
	for _, marker := range vcs.ParseMarkers(d.Notes[0].Body) {
		if marker.Kind != "finding" {
			continue
		}
		return firstRefPath(marker.Metadata.SourceMessageRefs)
	}
	return ""
}

func firstRefPath(refs []models.ReviewMessageRef) string {
	for _, ref := range refs {
		if ref.Path != "" {
			return ref.Path
		}
	}
	return ""
}

func codeContext(projectDir string, d gitlab.Discussion) string {
	for _, note := range d.Notes {
		if note.Position == nil {
			continue
		}
		path := note.Position.NewPath
		line := note.Position.NewLine
		if path == "" {
			path = note.Position.OldPath
			line = note.Position.OldLine
		}
		if path == "" {
			continue
		}
		data, err := os.ReadFile(projectDir + "/" + path) // #nosec G304 -- GitLab path in checked-out project
		if err != nil {
			return fmt.Sprintf("`%s` not found in current checkout.\n", path)
		}
		lines := strings.Split(string(data), "\n")
		start := max(1, line-8)
		end := min(len(lines), line+8)
		var b strings.Builder
		fmt.Fprintf(&b, "`%s:%d`\n\n```text\n", path, line)
		for i := start; i <= end; i++ {
			fmt.Fprintf(&b, "%4d %s\n", i, lines[i-1])
		}
		b.WriteString("```\n")
		return b.String()
	}
	return ""
}
