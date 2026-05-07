package diff

// FileDiff holds parsed information about a single changed file.
type FileDiff struct {
	Path       string
	OldPath    string
	Language   string
	ChangeType ChangeType
	Added      int
	Deleted    int
	Diff       string
}
