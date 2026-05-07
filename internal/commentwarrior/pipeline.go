package commentwarrior

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

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
		if err := p.git.Clean(); err != nil {
			slog.Warn("failed to clean working tree", "error", err)
		}
	}()

	if !p.cfg.DryRun {
		if err := p.validateHead(ctx); err != nil {
			return err
		}
	}
	me, err := p.gitlab.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}
	discussions, err := p.gitlab.ListDiscussions(ctx, p.cfg.MRIID)
	if err != nil {
		return err
	}

	resources, err := p.loader.LoadRuntime(ctx)
	if err != nil {
		return fmt.Errorf("load runtime: %w", err)
	}
	defer func() {
		if err := resources.Cleanup(); err != nil {
			slog.Warn("failed to cleanup runtime", "error", err)
		}
	}()

	if err := resources.Runner.StartServe(ctx); err != nil {
		return fmt.Errorf("start opencode: %w", err)
	}
	defer resources.Runner.StopServe()
	if err := resources.Runner.Precheck(ctx, "comment-warrior"); err != nil {
		return fmt.Errorf("precheck: %w", err)
	}

	processed := 0
	for _, d := range discussions {
		if p.cfg.DiscussionID != "" && d.ID != p.cfg.DiscussionID {
			continue
		}
		if p.cfg.MaxDiscussions > 0 && processed >= p.cfg.MaxDiscussions {
			break
		}
		classification := ClassifyDiscussion(d, me.ID)
		if !ShouldProcessDiscussion(classification, d, me.ID) {
			continue
		}
		task := resources.Message + "\n\n" + BuildTask(TaskConfig{Discussion: d, ProjectDir: p.cfg.ProjectDir})
		decision, err := runDecision(ctx, resources.Runner, task, d.ID)
		if err != nil {
			return err
		}
		closureConfirmation := classification == ClassAIFinding && discussionResolved(d)
		if p.cfg.DryRun {
			slog.Info("dry-run planned action", "discussion_id", d.ID, "action", decision.Action, "confidence", decision.Confidence, "reason", decision.Reason)
		} else if err := p.applyDecision(ctx, d.ID, *decision, closureConfirmation); err != nil {
			return err
		}
		processed++
	}
	slog.Info("comment-warrior completed", "processed", processed)
	return nil
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
