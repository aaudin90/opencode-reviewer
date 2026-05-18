package subagentconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	UseLegacyEnv      bool
	LegacyEnvFallback bool
}

// SubAgent holds a sub-agent's name and prompt content.
type SubAgent struct {
	Name   string
	Prompt string
}

// Load resolves sub-agent prompts with legacy env priority for backward compatibility.
// Use LoadWithOptions with LegacyEnvFallback for the CLI's deprecated env fallback mode.
func Load(envPathsKey, prefix, configDir string, tomlPaths, tomlInline []string) ([]SubAgent, error) {
	return LoadWithOptions(envPathsKey, prefix, configDir, tomlPaths, tomlInline, Options{UseLegacyEnv: true})
}

func LoadWithOptions(envPathsKey, prefix, configDir string, tomlPaths, tomlInline []string, opts Options) ([]SubAgent, error) {
	if opts.UseLegacyEnv && !opts.LegacyEnvFallback {
		if raw := os.Getenv(envPathsKey); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return readAll(splitPaths(raw), cwd)
		}
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
	if opts.UseLegacyEnv && opts.LegacyEnvFallback {
		if raw := os.Getenv(envPathsKey); raw != "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("get working directory: %w", err)
			}
			return readAll(splitPaths(raw), cwd)
		}
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

	hasListTools := strings.Contains(frontmatter, "\n  - type:")
	hasSubmitReview := hasDisabledTool(frontmatter, "submit_review")
	hasSubmitFinal := hasDisabledTool(frontmatter, "submit_final_review")

	if hasSubmitReview && hasSubmitFinal && !hasListTools {
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
		if hasListTools {
			blockEnd := findToolsBlockEnd(content, toolsAbsIdx+lineEndRel+1, fmStart+closeIdx)
			toolsBlock := content[toolsAbsIdx:blockEnd]
			normalized := normalizeListToolsBlock(toolsBlock)
			for _, line := range missing {
				toolName := strings.TrimSuffix(strings.TrimSpace(line), ": false")
				if !strings.Contains(normalized, "\n  "+toolName+": ") {
					normalized += "\n  " + toolName + ": false"
				}
			}
			return content[:toolsAbsIdx] + normalized + content[blockEnd:]
		}
		insertAt := toolsAbsIdx + lineEndRel
		return content[:insertAt] + "\n" + strings.Join(missing, "\n") + content[insertAt:]
	}

	insertAt := openIdx + 3 + closeIdx
	block := "tools:\n" + strings.Join(missing, "\n")
	return content[:insertAt] + "\n" + block + content[insertAt:]
}

func normalizeListToolsBlock(block string) string {
	type toolPermission struct {
		name    string
		allowed string
	}

	var result []toolPermission
	lines := strings.Split(block, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "- type: ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- type: "))
		allowed := "true"
		for _, next := range lines[i+1:] {
			nextTrimmed := strings.TrimSpace(next)
			if strings.HasPrefix(nextTrimmed, "- type: ") {
				break
			}
			if strings.HasPrefix(nextTrimmed, "allowed: ") {
				allowed = strings.TrimSpace(strings.TrimPrefix(nextTrimmed, "allowed: "))
				break
			}
		}
		result = append(result, toolPermission{name: name, allowed: allowed})
	}

	var b strings.Builder
	b.WriteString("tools:")
	for _, item := range result {
		b.WriteString("\n  ")
		b.WriteString(item.name)
		b.WriteString(": ")
		b.WriteString(item.allowed)
	}
	return b.String()
}

func hasDisabledTool(frontmatter, toolName string) bool {
	if strings.Contains(frontmatter, toolName+": false") {
		return true
	}

	lines := strings.Split(frontmatter, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "- type: "+toolName {
			continue
		}
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if strings.HasPrefix(trimmed, "- type: ") {
				return false
			}
			if trimmed == "allowed: false" {
				return true
			}
		}
	}
	return false
}

func findToolsBlockEnd(content string, start, frontmatterEnd int) int {
	pos := start
	for pos < frontmatterEnd {
		next := strings.IndexByte(content[pos:frontmatterEnd], '\n')
		lineEnd := frontmatterEnd
		if next >= 0 {
			lineEnd = pos + next
		}
		line := content[pos:lineEnd]
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			return pos
		}
		if next < 0 {
			break
		}
		pos = lineEnd + 1
	}
	return frontmatterEnd
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
