package diff

import "strings"

func parseFiles(rawDiff string) []FileDiff {
	if rawDiff == "" {
		return nil
	}

	sections := splitDiffSections(rawDiff)
	files := make([]FileDiff, 0, len(sections))

	for _, section := range sections {
		f := parseOneFile(section)
		if f.Path != "" {
			files = append(files, f)
		}
	}

	return files
}

func splitDiffSections(raw string) []string {
	const marker = "diff --git "

	idx := strings.Index(raw, marker)
	if idx < 0 {
		return nil
	}
	raw = raw[idx:]

	var sections []string
	for {
		next := strings.Index(raw[1:], marker)
		if next < 0 {
			sections = append(sections, raw)
			break
		}
		sections = append(sections, raw[:next+1])
		raw = raw[next+1:]
	}

	return sections
}

func parseOneFile(section string) FileDiff {
	lines := strings.Split(section, "\n")
	if len(lines) == 0 {
		return FileDiff{}
	}

	f := FileDiff{ChangeType: Modified}

	// Parse "diff --git a/path b/path"
	header := lines[0]
	if parts := parseDiffHeader(header); parts[0] != "" {
		f.Path = parts[1]
		f.OldPath = parts[0]
	}

	var added, deleted int

	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "new file mode"):
			f.ChangeType = Added
		case strings.HasPrefix(line, "deleted file mode"):
			f.ChangeType = Deleted
		case strings.HasPrefix(line, "rename from "):
			f.ChangeType = Renamed
			f.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			f.ChangeType = Renamed
			f.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			deleted++
		}
	}

	f.Added = added
	f.Deleted = deleted
	f.Diff = section
	f.Language = detectLanguage(f.Path)

	return f
}

// stripDiffHeaders removes per-file diff header lines (diff --git, index,
// ---, +++, mode changes, rename metadata), leaving only @@ hunk headers
// and actual change/context lines.
func stripDiffHeaders(raw string) string {
	var b strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
		case strings.HasPrefix(line, "index "):
		case strings.HasPrefix(line, "--- "):
		case strings.HasPrefix(line, "+++ "):
		case strings.HasPrefix(line, "old mode "):
		case strings.HasPrefix(line, "new mode "):
		case strings.HasPrefix(line, "new file mode "):
		case strings.HasPrefix(line, "deleted file mode "):
		case strings.HasPrefix(line, "similarity index "):
		case strings.HasPrefix(line, "rename from "):
		case strings.HasPrefix(line, "rename to "):
		default:
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
	}
	return b.String()
}

func parseDiffHeader(header string) [2]string {
	const prefix = "diff --git "
	if !strings.HasPrefix(header, prefix) {
		return [2]string{}
	}
	rest := header[len(prefix):]

	// Format: a/path b/path
	parts := strings.SplitN(rest, " b/", 2)
	if len(parts) != 2 {
		return [2]string{}
	}

	aPath := strings.TrimPrefix(parts[0], "a/")
	bPath := parts[1]

	return [2]string{aPath, bPath}
}
