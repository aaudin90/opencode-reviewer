package diff

// Result contains the full processed diff for a branch comparison.
type Result struct {
	Files                    []FileDiff
	FilteredFiles            []string
	TotalAdded, TotalDeleted int
	DiffStat, CommitLog      string
	Branch, BaseBranch       string
}
