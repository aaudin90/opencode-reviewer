package diff

// ChangeType represents the kind of change applied to a file.
type ChangeType string

const (
	Added    ChangeType = "added"
	Modified ChangeType = "modified"
	Deleted  ChangeType = "deleted"
	Renamed  ChangeType = "renamed"
)
