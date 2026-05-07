package gitlab

// DiffRefs holds the SHA references from a merge request diff version,
// used for positioning inline notes.
type DiffRefs struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
}
