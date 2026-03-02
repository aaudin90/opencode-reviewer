package subagentconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SubAgent holds a sub-agent's name and prompt content.
type SubAgent struct {
	Name   string
	Prompt string
}

// Load resolves sub-agent prompts by priority:
//  1. envPathsKey env var (comma-separated file paths, relative to cwd) → read files → return SubAgents with filename-based names
//  2. tomlInline (if non-empty) → return SubAgents with generated names "sub-<prefix>-1", "sub-<prefix>-2", ...
//  3. tomlPaths (relative to configDir) → read files → return SubAgents with filename-based names
//  4. nil (no sub-agents configured)
func Load(envPathsKey, prefix, configDir string, tomlPaths, tomlInline []string) ([]SubAgent, error) {
	if raw := os.Getenv(envPathsKey); raw != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		return readAll(splitPaths(raw), cwd)
	}
	if len(tomlInline) > 0 {
		agents := make([]SubAgent, len(tomlInline))
		for i, prompt := range tomlInline {
			name := fmt.Sprintf("sub-%s-%d", prefix, i+1)
			if err := validate(name, prompt); err != nil {
				return nil, err
			}
			agents[i] = SubAgent{
				Name:   name,
				Prompt: ensureToolRestrictions(prompt),
			}
		}
		return agents, nil
	}
	if len(tomlPaths) > 0 {
		return readAll(tomlPaths, configDir)
	}
	return nil, nil
}

func splitPaths(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

func validate(name, content string) error {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return fmt.Errorf("sub-agent %q: missing YAML frontmatter (must start with ---)", name)
	}
	rest := trimmed[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fmt.Errorf("sub-agent %q: missing closing --- in YAML frontmatter", name)
	}
	frontmatter := rest[:idx]
	if !strings.Contains(frontmatter, "mode: subagent") && !strings.Contains(frontmatter, "mode:subagent") {
		return fmt.Errorf("sub-agent %q: frontmatter must contain 'mode: subagent'", name)
	}
	return nil
}

// ensureToolRestrictions injects missing tool restrictions into the YAML frontmatter.
// If submit_review: false or submit_final_review: false is missing, they are added
// under the tools: section (created if absent). This is a safety-net to prevent
// sub-agents from calling terminal tools that end the review session.
func ensureToolRestrictions(content string) string {
	openIdx := strings.Index(content, "---")
	afterOpen := content[openIdx+3:]
	closeIdx := strings.Index(afterOpen, "\n---")
	frontmatter := afterOpen[:closeIdx]

	hasSubmitReview := strings.Contains(frontmatter, "submit_review: false")
	hasSubmitFinal := strings.Contains(frontmatter, "submit_final_review: false")

	if hasSubmitReview && hasSubmitFinal {
		return content
	}

	var missing []string
	if !hasSubmitReview {
		missing = append(missing, "  submit_review: false")
	}
	if !hasSubmitFinal {
		missing = append(missing, "  submit_final_review: false")
	}

	if strings.Contains(frontmatter, "tools:") {
		fmStart := openIdx + 3
		toolsRelIdx := strings.Index(frontmatter, "tools:")
		toolsAbsIdx := fmStart + toolsRelIdx
		lineEndRel := strings.Index(content[toolsAbsIdx:], "\n")
		insertAt := toolsAbsIdx + lineEndRel
		return content[:insertAt] + "\n" + strings.Join(missing, "\n") + content[insertAt:]
	}

	insertAt := openIdx + 3 + closeIdx
	block := "tools:\n" + strings.Join(missing, "\n")
	return content[:insertAt] + "\n" + block + content[insertAt:]
}

func readAll(paths []string, baseDir string) ([]SubAgent, error) {
	result := make([]SubAgent, 0, len(paths))
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(baseDir, p)
		}
		data, err := os.ReadFile(filepath.Clean(abs)) // #nosec G304 G703 -- path from trusted config or env
		if err != nil {
			return nil, fmt.Errorf("read sub-agent prompt %q: %w", abs, err)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		if err := validate(name, string(data)); err != nil {
			return nil, err
		}
		prompt := ensureToolRestrictions(string(data))
		result = append(result, SubAgent{Name: name, Prompt: prompt})
	}
	return result, nil
}
