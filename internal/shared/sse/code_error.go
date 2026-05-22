package sse

import (
	"errors"
	"fmt"
	"strings"
)

// CodeError represents an opencode HTTP/session error with a concrete 4xx/5xx status.
type CodeError struct {
	StatusCode int
	Source     string
	Snippet    string
}

func (e *CodeError) Error() string {
	if e.Snippet == "" {
		return fmt.Sprintf("%s returned status %d", e.Source, e.StatusCode)
	}
	return fmt.Sprintf("%s returned status %d: %s", e.Source, e.StatusCode, e.Snippet)
}

func newCodeError(status int, source, snippet string) error {
	return &CodeError{
		StatusCode: status,
		Source:     source,
		Snippet:    sanitizeSnippet(snippet, 512),
	}
}

// IsCodeError reports whether err is a structured 4xx/5xx opencode error.
func IsCodeError(err error) bool {
	var codeErr *CodeError
	return errors.As(err, &codeErr)
}

func sanitizeSnippet(value string, maxLen int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}

func isHTTPCodeError(status int) bool {
	return status >= 400 && status <= 599
}
