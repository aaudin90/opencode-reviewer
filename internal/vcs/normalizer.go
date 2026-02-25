package vcs

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/models"
)

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Normalizer corrects start_line / end_line in findings by matching
// existing_code against the actual diff content.
type Normalizer struct {
	fileLines    map[string][]DiffLine
	fileOldPaths map[string]string // new path → old path (only for renamed files)
}

// NewNormalizer parses diff hunks from a diff.Result and builds per-file
// DiffLine slices used for line number correction.
func NewNormalizer(diffResult *diff.Result) *Normalizer {
	fileLines := make(map[string][]DiffLine, len(diffResult.Files))
	fileOldPaths := make(map[string]string)
	for _, f := range diffResult.Files {
		lines := parseDiffLines(f.Diff)
		if len(lines) > 0 {
			fileLines[f.Path] = lines
			if f.OldPath != "" && f.OldPath != f.Path {
				fileOldPaths[f.Path] = f.OldPath
			}
		}
	}
	return &Normalizer{fileLines: fileLines, fileOldPaths: fileOldPaths}
}

// Normalize returns a new slice of findings with corrected line numbers.
// The original slice is not mutated.
func (n *Normalizer) Normalize(findings []models.FinalFinding) []models.FinalFinding {
	out := make([]models.FinalFinding, len(findings))
	copy(out, findings)

	var normalized, unchanged, unresolved int

	for i := range out {
		f := &out[i]

		if strings.TrimSpace(f.ExistingCode) == "" {
			unchanged++
			continue
		}

		_, diffLines, ok := n.resolveFileLines(f.File)
		if !ok {
			unchanged++
			continue
		}

		if n.verifyAtPosition(diffLines, f) {
			unchanged++
			continue
		}

		if n.searchAndCorrect(diffLines, f) {
			normalized++
		} else {
			unresolved++
			slog.Warn("normalizer: existing_code not found in diff",
				"file", f.File,
				"start_line", f.StartLine,
				"existing_code_prefix", truncate(f.ExistingCode, 60),
			)
		}
	}

	slog.Info("normalization complete",
		"normalized", normalized,
		"unchanged", unchanged,
		"unresolved", unresolved,
	)

	return out
}

// verifyAtPosition checks whether existing_code matches lines at the
// finding's current start_line (trying NewNum first, then OldNum).
func (n *Normalizer) verifyAtPosition(lines []DiffLine, f *models.FinalFinding) bool {
	codeLines := splitCode(f.ExistingCode)

	// Try NewNum positions first (added or context line).
	if matchAtLine(lines, codeLines, f.StartLine, func(dl DiffLine) int { return dl.NewNum }) {
		return true
	}
	// Try OldNum positions (deleted line).
	if matchAtLine(lines, codeLines, f.StartLine, func(dl DiffLine) int { return dl.OldNum }) {
		return true
	}
	return false
}

// searchAndCorrect performs a content-based search for existing_code in the
// diff lines, then corrects the finding's start/end lines.
func (n *Normalizer) searchAndCorrect(lines []DiffLine, f *models.FinalFinding) bool {
	codeLines := splitCode(f.ExistingCode)
	if len(codeLines) == 0 {
		return false
	}

	type match struct {
		idx  int
		dist int
	}

	var matches []match

	if len(codeLines) == 1 {
		trimmed := strings.TrimSpace(codeLines[0])
		for i, dl := range lines {
			if strings.TrimSpace(dl.Content) == trimmed {
				matches = append(matches, match{idx: i, dist: absDist(lineNum(dl), f.StartLine)})
			}
		}
	} else {
		for i := 0; i <= len(lines)-len(codeLines); i++ {
			if windowMatch(lines[i:i+len(codeLines)], codeLines) {
				matches = append(matches, match{idx: i, dist: absDist(lineNum(lines[i]), f.StartLine)})
			}
		}
	}

	if len(matches) == 0 {
		return false
	}

	best := matches[0]
	for _, m := range matches[1:] {
		if m.dist < best.dist {
			best = m
		}
	}

	start := lines[best.idx]
	end := lines[best.idx+len(codeLines)-1]
	f.StartLine = lineNum(start)
	f.EndLine = lineNum(end)

	return true
}

// NormalizeDiff corrects OldLine/NewLine and OldPath/NewPath in a slice of
// DiffFindings by matching Source.ExistingCode against the actual diff content.
// The original slice is not mutated.
func (n *Normalizer) NormalizeDiff(findings []DiffFinding) []DiffFinding {
	out := make([]DiffFinding, len(findings))
	copy(out, findings)

	var normalized, unchanged, unresolved int

	for i := range out {
		df := &out[i]

		// Always resolve actual old/new paths from diff (works even for empty ExistingCode).
		resolvedKey, diffLines, pathOK := n.resolveFileLines(df.Source.File)
		if pathOK {
			df.NewPath = resolvedKey
			if oldPath, hasOld := n.fileOldPaths[resolvedKey]; hasOld {
				df.OldPath = oldPath
			} else {
				df.OldPath = resolvedKey
			}
		}

		if strings.TrimSpace(df.Source.ExistingCode) == "" {
			unchanged++
			if pathOK {
				df.InDiff = isLineInDiff(diffLines, df.OldLine, df.NewLine)
			}
			continue
		}
		if !pathOK {
			unchanged++
			continue
		}

		if n.verifyDiffAtPosition(diffLines, df) {
			df.InDiff = true
			unchanged++
			continue
		}

		if n.searchAndCorrectDiff(diffLines, df) {
			df.InDiff = true
			normalized++
		} else {
			unresolved++
			slog.Warn("normalizer: existing_code not found in diff",
				"file", df.Source.File,
				"start_line", df.Source.StartLine,
				"existing_code_prefix", truncate(df.Source.ExistingCode, 60),
			)
		}
	}

	slog.Info("diff normalization complete",
		"normalized", normalized,
		"unchanged", unchanged,
		"unresolved", unresolved,
	)

	return out
}

// verifyDiffAtPosition checks whether Source.ExistingCode matches lines at
// Source.StartLine (trying NewNum first, then OldNum).
// Sets df.NewLine or df.OldLine to indicate which side matched.
func (n *Normalizer) verifyDiffAtPosition(lines []DiffLine, df *DiffFinding) bool {
	codeLines := splitCode(df.Source.ExistingCode)
	startLine := df.Source.StartLine

	// Try NewNum positions first (added or context line).
	if matchAtLine(lines, codeLines, startLine, func(dl DiffLine) int { return dl.NewNum }) {
		df.OldLine = 0
		df.NewLine = startLine
		return true
	}
	// Try OldNum positions (deleted line).
	if matchAtLine(lines, codeLines, startLine, func(dl DiffLine) int { return dl.OldNum }) {
		df.NewLine = 0
		df.OldLine = startLine
		return true
	}
	return false
}

// searchAndCorrectDiff performs a content-based search for Source.ExistingCode
// in the diff lines and sets df.OldLine/NewLine based on the matched position.
func (n *Normalizer) searchAndCorrectDiff(lines []DiffLine, df *DiffFinding) bool {
	codeLines := splitCode(df.Source.ExistingCode)
	if len(codeLines) == 0 {
		return false
	}

	type match struct {
		idx  int
		dist int
	}

	var matches []match

	if len(codeLines) == 1 {
		trimmed := strings.TrimSpace(codeLines[0])
		for i, dl := range lines {
			if strings.TrimSpace(dl.Content) == trimmed {
				matches = append(matches, match{idx: i, dist: absDist(lineNum(dl), df.Source.StartLine)})
			}
		}
	} else {
		for i := 0; i <= len(lines)-len(codeLines); i++ {
			if windowMatch(lines[i:i+len(codeLines)], codeLines) {
				matches = append(matches, match{idx: i, dist: absDist(lineNum(lines[i]), df.Source.StartLine)})
			}
		}
	}

	if len(matches) == 0 {
		return false
	}

	best := matches[0]
	for _, m := range matches[1:] {
		if m.dist < best.dist {
			best = m
		}
	}

	start := lines[best.idx]
	if start.Origin == "-" {
		df.OldLine = start.OldNum
		df.NewLine = 0
	} else {
		df.NewLine = start.NewNum
		df.OldLine = 0
	}

	return true
}

// resolveFileLines looks up the DiffLine slice for filePath.
// Returns the resolved diff key (canonical file path), the DiffLine slice, and a bool.
//
// Resolution order:
//  1. Exact match.
//  2. Diff key ends with finding path (LLM omitted directory prefix,
//     e.g. finding="main.go", diff="src/main.go").
//  3. Strip leading segments from finding path and repeat (LLM added extra
//     prefix, e.g. finding="a/src/main.go", diff="src/main.go").
func (n *Normalizer) resolveFileLines(filePath string) (string, []DiffLine, bool) {
	if lines, ok := n.fileLines[filePath]; ok {
		return filePath, lines, true
	}

	clean := strings.TrimPrefix(filePath, "/")

	// Check if any diff key ends with the full (cleaned) finding path.
	for key, lines := range n.fileLines {
		if strings.HasSuffix(key, "/"+clean) {
			slog.Debug("normalizer: resolved file by suffix match",
				"finding_file", filePath,
				"diff_file", key,
			)
			return key, lines, true
		}
	}

	// Strip leading segments from finding path one at a time and retry.
	parts := strings.Split(clean, "/")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		for key, lines := range n.fileLines {
			if key == suffix || strings.HasSuffix(key, "/"+suffix) {
				slog.Debug("normalizer: resolved file by suffix match",
					"finding_file", filePath,
					"diff_file", key,
				)
				return key, lines, true
			}
		}
	}

	return "", nil, false
}

// isLineInDiff reports whether the given old or new line number appears in
// the parsed diff lines (i.e. is part of a diff hunk).
func isLineInDiff(lines []DiffLine, oldLine, newLine int) bool {
	for _, dl := range lines {
		if newLine > 0 && dl.NewNum == newLine {
			return true
		}
		if oldLine > 0 && dl.OldNum == oldLine {
			return true
		}
	}
	return false
}

// parseDiffLines extracts DiffLine entries from a raw unified diff string,
// skipping diff headers and parsing hunk headers for line numbering.
func parseDiffLines(raw string) []DiffLine {
	var result []DiffLine
	var oldNum, newNum int

	for _, line := range strings.Split(raw, "\n") {
		if isHeader(line) {
			continue
		}

		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			oldNum, _ = strconv.Atoi(m[1])
			newNum, _ = strconv.Atoi(m[2])
			continue
		}

		if len(line) == 0 {
			// Empty line in diff — context line where git omits the leading space.
			result = append(result, DiffLine{
				Content: "",
				OldNum:  oldNum,
				NewNum:  newNum,
				Origin:  " ",
			})
			oldNum++
			newNum++
			continue
		}

		switch line[0] {
		case '+':
			result = append(result, DiffLine{
				Content: line[1:],
				OldNum:  0,
				NewNum:  newNum,
				Origin:  "+",
			})
			newNum++
		case '-':
			result = append(result, DiffLine{
				Content: line[1:],
				OldNum:  oldNum,
				NewNum:  0,
				Origin:  "-",
			})
			oldNum++
		default:
			// Context line (starts with space or any other character).
			content := line
			if line[0] == ' ' {
				content = line[1:]
			}
			result = append(result, DiffLine{
				Content: content,
				OldNum:  oldNum,
				NewNum:  newNum,
				Origin:  " ",
			})
			oldNum++
			newNum++
		}
	}

	return result
}

// isHeader returns true for diff header lines that should be skipped.
func isHeader(line string) bool {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		return true
	case strings.HasPrefix(line, "index "):
		return true
	case strings.HasPrefix(line, "--- "):
		return true
	case strings.HasPrefix(line, "+++ "):
		return true
	case strings.HasPrefix(line, "old mode "):
		return true
	case strings.HasPrefix(line, "new mode "):
		return true
	case strings.HasPrefix(line, "new file mode "):
		return true
	case strings.HasPrefix(line, "deleted file mode "):
		return true
	case strings.HasPrefix(line, "similarity index "):
		return true
	case strings.HasPrefix(line, "rename from "):
		return true
	case strings.HasPrefix(line, "rename to "):
		return true
	}
	return false
}

// splitCode splits existing_code into non-empty lines.
func splitCode(code string) []string {
	raw := strings.Split(code, "\n")
	var out []string
	for _, l := range raw {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// matchAtLine checks if codeLines match at startLine using the given line number accessor.
func matchAtLine(lines []DiffLine, codeLines []string, startLine int, numFn func(DiffLine) int) bool {
	startIdx := -1
	for i, dl := range lines {
		if numFn(dl) == startLine {
			startIdx = i
			break
		}
	}
	if startIdx < 0 || startIdx+len(codeLines) > len(lines) {
		return false
	}
	return windowMatch(lines[startIdx:startIdx+len(codeLines)], codeLines)
}

// windowMatch checks whether a slice of DiffLines matches code lines (trimmed comparison).
func windowMatch(window []DiffLine, codeLines []string) bool {
	for i, cl := range codeLines {
		if strings.TrimSpace(window[i].Content) != strings.TrimSpace(cl) {
			return false
		}
	}
	return true
}

// lineNum returns the effective line number for a DiffLine:
// "+" → NewNum, "-" → OldNum, " " → NewNum.
func lineNum(dl DiffLine) int {
	switch dl.Origin {
	case "+":
		return dl.NewNum
	case "-":
		return dl.OldNum
	default:
		return dl.NewNum
	}
}

func absDist(a, b int) int {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
