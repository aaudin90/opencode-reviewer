package promptref

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/models"
)

// FromContent creates a stable reference for inline prompt content.
func FromContent(id, content string) models.ReviewMessageRef {
	return models.ReviewMessageRef{
		ID:     id,
		SHA256: hash(content),
	}
}

// LoadReviewMessages reads file-backed and inline reviewer messages and returns
// content with compact references.
func LoadReviewMessages(baseDir string, paths []string, inline []string) ([]models.ReviewMessage, error) {
	if len(inline) > 0 {
		result := make([]models.ReviewMessage, 0, len(inline))
		for i, content := range inline {
			id := fmt.Sprintf("inline-%d", i+1)
			result = append(result, models.ReviewMessage{Ref: FromContent(id, content), Content: content})
		}
		return result, nil
	}

	result := make([]models.ReviewMessage, 0, len(paths))
	for i, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(baseDir, p)
		}
		data, err := os.ReadFile(filepath.Clean(abs)) // #nosec G304 G703 -- trusted config path
		if err != nil {
			return nil, fmt.Errorf("read message file %q: %w", abs, err)
		}
		content := string(data)
		result = append(result, models.ReviewMessage{
			Ref: models.ReviewMessageRef{
				ID:     strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)),
				Path:   abs,
				SHA256: hash(content),
			},
			Content: content,
		})
		_ = i
	}
	return result, nil
}

// MatchExact returns the message whose ref matches all non-empty fields in ref.
func MatchExact(messages []models.ReviewMessage, ref models.ReviewMessageRef) *models.ReviewMessage {
	for i := range messages {
		got := messages[i].Ref
		if ref.ID != "" && got.ID != ref.ID {
			continue
		}
		if ref.Path != "" && got.Path != ref.Path {
			continue
		}
		if ref.SHA256 != "" && got.SHA256 != ref.SHA256 {
			continue
		}
		return &messages[i]
	}
	return nil
}

// ReferenceOnlyXML wraps a prompt reference for task payloads without including
// prompt content.
func ReferenceOnlyXML(ref models.ReviewMessageRef) string {
	var b strings.Builder
	b.WriteString("<review_message_ref")
	if ref.ID != "" {
		fmt.Fprintf(&b, " id=%q", ref.ID)
	}
	if ref.Path != "" {
		fmt.Fprintf(&b, " path=%q", ref.Path)
	}
	if ref.SHA256 != "" {
		fmt.Fprintf(&b, " sha256=%q", ref.SHA256)
	}
	b.WriteString(" />")
	return b.String()
}

func hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
