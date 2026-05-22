package pipeline

import (
	"context"
	"errors"
)

func normalizeModelChain(models []string) []string {
	if len(models) == 0 {
		return []string{""}
	}
	return models
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func modelLabel(model string) string {
	if model == "" {
		return "<default>"
	}
	return model
}
