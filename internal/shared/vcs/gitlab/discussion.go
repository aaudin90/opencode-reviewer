package gitlab

type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type Position struct {
	PositionType string `json:"position_type"`
	BaseSHA      string `json:"base_sha"`
	HeadSHA      string `json:"head_sha"`
	StartSHA     string `json:"start_sha"`
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	OldLine      int    `json:"old_line"`
	NewLine      int    `json:"new_line"`
}

type Note struct {
	ID         int       `json:"id"`
	Body       string    `json:"body"`
	System     bool      `json:"system"`
	Resolvable bool      `json:"resolvable"`
	Resolved   bool      `json:"resolved"`
	Author     Author    `json:"author"`
	Position   *Position `json:"position"`
}

type Discussion struct {
	ID             string `json:"id"`
	IndividualNote bool   `json:"individual_note"`
	Notes          []Note `json:"notes"`
}

// clearable reports whether this discussion should be deleted:
// a single note with no replies, not system-generated and not already resolved.
func (d Discussion) clearable() bool {
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
