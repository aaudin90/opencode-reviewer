package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/sse"
)

type codeError struct {
	status  int
	source  string
	snippet string
}

func (e *codeError) Error() string {
	if e.snippet == "" {
		return fmt.Sprintf("%s returned status %d", e.source, e.status)
	}
	return fmt.Sprintf("%s returned status %d: %s", e.source, e.status, e.snippet)
}

func newCodeError(status int, source, snippet string) error {
	return &codeError{
		status:  status,
		source:  source,
		snippet: sanitizeErrorSnippet(snippet, 512),
	}
}

func isCodeError(err error) bool {
	if err == nil {
		return false
	}
	var local *codeError
	return errors.As(err, &local) || sse.IsCodeError(err)
}

func isHTTPCodeError(status int) bool {
	return status >= 400 && status <= 599
}

func sanitizeErrorSnippet(value string, maxLen int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}
