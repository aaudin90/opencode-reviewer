package gitlab

type noteAuthor struct {
	ID int `json:"id"`
}

type discussionNote struct {
	ID         int        `json:"id"`
	System     bool       `json:"system"`
	Resolvable bool       `json:"resolvable"`
	Resolved   bool       `json:"resolved"`
	Author     noteAuthor `json:"author"`
}

type mrDiscussion struct {
	ID             string           `json:"id"`
	IndividualNote bool             `json:"individual_note"`
	Notes          []discussionNote `json:"notes"`
}

// clearable reports whether this discussion should be deleted:
// a single note with no replies, not system-generated and not already resolved.
func (d mrDiscussion) clearable() bool {
	if len(d.Notes) != 1 {
		return false
	}
	n := d.Notes[0]
	if n.System {
		return false
	}
	if n.Resolvable && n.Resolved {
		return false
	}
	return true
}
