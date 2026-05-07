package commentwarrior

import "github.com/aaudin90/opencode-reviewer/internal/shared/vcs/gitlab"

type processableDiscussion struct {
	discussion     gitlab.Discussion
	classification Classification
}
