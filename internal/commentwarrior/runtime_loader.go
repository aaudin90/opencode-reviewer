package commentwarrior

import (
	"context"

	commentwarriorruntime "github.com/aaudin90/opencode-reviewer/internal/commentwarrior/runtime"
)

type RuntimeLoader interface {
	LoadRuntime(context.Context) (*commentwarriorruntime.RuntimeResources, error)
}
