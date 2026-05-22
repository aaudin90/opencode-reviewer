package commentwarrior

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	commentwarriorruntime "github.com/aaudin90/opencode-reviewer/internal/commentwarrior/runtime"
	"github.com/aaudin90/opencode-reviewer/internal/shared/diff"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
	"github.com/aaudin90/opencode-reviewer/internal/shared/runner"
	"github.com/aaudin90/opencode-reviewer/internal/shared/vcs"
	gitlabvcs "github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"
)

type Pipeline struct {
	cfg    PipelineConfig
	git    *git.Client
	gitlab *gitlabvcs.Client
	loader RuntimeLoader
	runner *runner.Runner
}

func NewPipeline(cfg PipelineConfig, gitClient *git.Client, gitlabClient *gitlabvcs.Client, loader RuntimeLoader) *Pipeline {
	return &Pipeline{
		cfg:    cfg,
		git:    gitClient,
		gitlab: gitlabClient,
		loader: loader,
	}
}

func (p *Pipeline) Run(ctx context.Context) error {
	if err := git.PrepareRepository(p.git, p.cfg.Branch); err != nil {
		return fmt.Errorf("prepare repository: %w", err)
	}
	defer func() {
		if err := p.git.Clean(p.logDirs()...); err != nil {
			slog.Warn("failed to clean working tree", "error", err)
		}
	}()

	processable, err := p.processableDiscussions(ctx)
	if err != nil {
		return err
	}
	if len(processable) == 0 {
		slog.Info("comment-warrior completed", "processed", 0)
		return nil
	}

	if !p.cfg.DryRun {
		if err := p.validateHead(ctx); err != nil {
			return err
		}
	}

	processed, err := p.runProcessableDiscussions(ctx, processable)
	if err != nil {
		return err
	}
	slog.Info("comment-warrior completed", "processed", processed)
	return nil
}

func (p *Pipeline) LogPaths() []string {
	if p.runner == nil || p.runner.LogPath() == "" {
		return nil
	}
	return []string{p.runner.LogPath()}
}

func (p *Pipeline) logDirs() []string {
	paths := p.LogPaths()
	if len(paths) == 0 {
		return nil
	}
	return []string{filepath.Dir(paths[0])}
}

func (p *Pipeline) processableDiscussions(ctx context.Context) ([]processableDiscussion, error) {
	me, err := p.gitlab.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	discussions, err := p.gitlab.ListDiscussions(ctx, p.cfg.MRIID)
	if err != nil {
		return nil, err
	}
	return p.filterProcessableDiscussions(discussions, me.ID), nil
}

func (p *Pipeline) runProcessableDiscussions(ctx context.Context, processable []processableDiscussion) (int, error) {
	resources, err := p.loader.LoadRuntime(ctx)
	if err != nil {
		return 0, fmt.Errorf("load runtime: %w", err)
	}
	defer func() {
		if err := resources.Cleanup(); err != nil {
			slog.Warn("failed to cleanup runtime", "error", err)
		}
	}()

	if err := resources.Runner.StartServe(ctx); err != nil {
		return 0, fmt.Errorf("start opencode: %w", err)
	}
	p.runner = resources.Runner
	defer resources.Runner.StopServe()
	if err := resources.Runner.Precheck(ctx, "comment-warrior", ""); err != nil {
		return 0, fmt.Errorf("precheck: %w", err)
	}
	diffPath, err := p.prepareDiff()
	if err != nil {
		return 0, fmt.Errorf("prepare diff: %w", err)
	}

	processed := 0
	for _, item := range processable {
		if err := p.processDiscussion(ctx, resources, item, diffPath); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (p *Pipeline) processDiscussion(ctx context.Context, resources *commentwarriorruntime.RuntimeResources, item processableDiscussion, diffPath string) error {
	d := item.discussion
	classification := item.classification
	sourcePromptPath := sourceReviewPromptPath(d)
	sourcePrompt := sourceReviewPrompt(p.cfg.ProjectDir, d)
	slog.Info("prepared comment-warrior task",
		"discussion_id", d.ID,
		"classification", classification,
		"message_kind", messageKind(classification),
		"source_review_prompt_path", sourcePromptPath,
		"source_review_prompt_loaded", sourcePrompt != "",
		"source_review_prompt_bytes", len(sourcePrompt),
	)
	task := buildPrompt(
		messageForClassification(resources, classification),
		sourcePrompt,
		buildTask(taskConfig{
			Discussion:         d,
			ProjectDir:         p.cfg.ProjectDir,
			DiffPath:           diffPath,
			SourceReviewPrompt: sourcePrompt,
		}),
	)
	decision, err := runDecision(ctx, resources.Runner, task, d.ID)
	if err != nil {
		return err
	}
	closureConfirmation := shouldConfirmClosure(classification, d, *decision)
	if p.cfg.DryRun {
		slog.Info("dry-run planned action", "discussion_id", d.ID, "action", decision.Action, "confidence", decision.Confidence, "reason", decision.Reason)
		return nil
	}
	return p.applyDecision(ctx, d.ID, *decision, closureConfirmation)
}

func (p *Pipeline) filterProcessableDiscussions(discussions []gitlabvcs.Discussion, botUserID int) []processableDiscussion {
	result := make([]processableDiscussion, 0, len(discussions))
	for _, d := range discussions {
		if p.cfg.DiscussionID != "" && d.ID != p.cfg.DiscussionID {
			continue
		}
		if p.cfg.MaxDiscussions > 0 && len(result) >= p.cfg.MaxDiscussions {
			break
		}
		classification := ClassifyDiscussion(d, botUserID)
		if !ShouldProcessDiscussion(classification, d, botUserID) {
			continue
		}
		result = append(result, processableDiscussion{
			discussion:     d,
			classification: classification,
		})
	}
	return result
}

func messageForClassification(resources *commentwarriorruntime.RuntimeResources, classification Classification) string {
	if classification == ClassHumanMentionAI {
		return resources.MentionMessage
	}
	return resources.FindingMessage
}

func buildPrompt(message, sourcePrompt, task string) string {
	parts := []string{message}
	if sourcePrompt != "" {
		parts = append(parts, "A <source_review_prompt> block is attached below. Use it as context for the review rules that produced the original AI finding.")
	}
	parts = append(parts, task)
	return strings.Join(parts, "\n\n")
}

func messageKind(classification Classification) string {
	if classification == ClassHumanMentionAI {
		return "mention"
	}
	return "finding"
}

func shouldConfirmClosure(classification Classification, discussion gitlabvcs.Discussion, decision Decision) bool {
	if classification != ClassAIFinding {
		return false
	}
	if discussionResolved(discussion) {
		return decision.Action == ActionReply || decision.Action == ActionResolve
	}
	return decision.Action == ActionResolve
}

func runDecision(ctx context.Context, r *runner.Runner, prompt, discussionID string) (*Decision, error) {
	var result *runner.RunResult
	for event := range r.Run(ctx, runner.RunRequest{
		Prompt:     prompt,
		ToolName:   "submit_comment_warrior_decision",
		PromptPath: "discussion-" + discussionID,
		AgentName:  "comment-warrior",
		ValidateFunc: func(data json.RawMessage) error {
			_, err := ParseDecision(data)
			return err
		},
		SchemaHint: DecisionSchemaHint,
	}) {
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Final != nil {
			result = event.Final
		}
	}
	if result == nil || result.ToolArgs == nil {
		return nil, fmt.Errorf("no comment-warrior decision received")
	}
	return ParseDecision(result.ToolArgs)
}

func (p *Pipeline) applyDecision(ctx context.Context, discussionID string, d Decision, closureConfirmation bool) error {
	switch d.Action {
	case ActionReply:
		return p.reply(ctx, discussionID, d.Body, closureConfirmation)
	case ActionResolve:
		if d.Body != "" {
			if err := p.reply(ctx, discussionID, d.Body, closureConfirmation); err != nil {
				return err
			}
		}
		return p.gitlab.SetDiscussionResolved(ctx, p.cfg.MRIID, discussionID, true)
	case ActionUnresolve:
		if d.Body != "" {
			if err := p.reply(ctx, discussionID, d.Body, false); err != nil {
				return err
			}
		}
		return p.gitlab.SetDiscussionResolved(ctx, p.cfg.MRIID, discussionID, false)
	case ActionNoop:
		return nil
	default:
		return fmt.Errorf("unsupported action %q", d.Action)
	}
}

func (p *Pipeline) reply(ctx context.Context, discussionID, body string, closureConfirmation bool) error {
	if closureConfirmation {
		body = vcs.AppendMarker(body, ClosureConfirmedMarkerKind, vcs.MarkerMetadata{})
	}
	return p.gitlab.ReplyToDiscussion(ctx, p.cfg.MRIID, discussionID, body)
}

func (p *Pipeline) prepareDiff() (string, error) {
	slog.Info("preparing comment-warrior diff context", "branch", p.cfg.Branch, "base", p.cfg.BaseBranch)
	result, err := diff.Prepare(p.git, p.cfg.Branch, p.cfg.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("prepare: %w", err)
	}
	slog.Info("comment-warrior diff parsed",
		"files", len(result.Files),
		"filtered", len(result.FilteredFiles),
		"added", result.TotalAdded,
		"deleted", result.TotalDeleted,
		"tokens_estimate", diff.EstimateTokens(result),
	)
	path, err := diff.WriteContextFile(result, p.git.Dir())
	if err != nil {
		return "", fmt.Errorf("write context file: %w", err)
	}
	slog.Info("comment-warrior diff context ready", "path", path)
	return path, nil
}

func (p *Pipeline) validateHead(ctx context.Context) error {
	info, err := p.gitlab.GetMergeRequest(ctx, p.cfg.MRIID)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = p.cfg.ProjectDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	head := strings.TrimSpace(string(out))
	if info.SHA != "" && head != info.SHA {
		return fmt.Errorf("local checkout head %s does not match GitLab MR head %s", head, info.SHA)
	}
	return nil
}
