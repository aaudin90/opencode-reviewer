package vcs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

const markerPrefix = "opencode-reviewer"

type MarkerMetadata struct {
	ID                  string                    `json:"id,omitempty"`
	BaseSHA             string                    `json:"base_sha,omitempty"`
	HeadSHA             string                    `json:"head_sha,omitempty"`
	StartSHA            string                    `json:"start_sha,omitempty"`
	File                string                    `json:"file,omitempty"`
	StartLine           int                       `json:"start_line,omitempty"`
	EndLine             int                       `json:"end_line,omitempty"`
	OldPath             string                    `json:"old_path,omitempty"`
	NewPath             string                    `json:"new_path,omitempty"`
	OldLine             int                       `json:"old_line,omitempty"`
	NewLine             int                       `json:"new_line,omitempty"`
	SourceMessageRefs   []models.ReviewMessageRef `json:"source_message_refs,omitempty"`
	FallbackMessageRefs []models.ReviewMessageRef `json:"fallback_message_refs,omitempty"`
}

type ParsedMarker struct {
	Kind     string
	Metadata MarkerMetadata
}

var markerRE = regexp.MustCompile(`<!--\s*opencode-reviewer:([a-z-]+):v1:([A-Za-z0-9_-]+)\s*-->`)

func AppendMarker(body, kind string, metadata MarkerMetadata) string {
	data, err := json.Marshal(metadata)
	if err != nil {
		return body
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	return fmt.Sprintf("%s\n\n<!-- %s:%s:v1:%s -->", body, markerPrefix, kind, encoded)
}

func ParseMarkers(body string) []ParsedMarker {
	matches := markerRE.FindAllStringSubmatch(body, -1)
	result := make([]ParsedMarker, 0, len(matches))
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		data, err := base64.RawURLEncoding.DecodeString(match[2])
		if err != nil {
			continue
		}
		var metadata MarkerMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			continue
		}
		result = append(result, ParsedMarker{Kind: match[1], Metadata: metadata})
	}
	return result
}

func StripMarkers(body string) string {
	return markerRE.ReplaceAllString(body, "")
}
