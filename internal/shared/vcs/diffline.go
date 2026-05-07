package vcs

// DiffLine represents a single line within a unified diff hunk.
type DiffLine struct {
	Content string // line text without the +/-/space prefix
	OldNum  int    // 0 if the line was added (+)
	NewNum  int    // 0 if the line was deleted (-)
	Origin  string // "+" | "-" | " "
}
