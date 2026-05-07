package models

// ReviewMessageRef identifies a reviewer prompt/message without carrying its
// full content. It is safe to persist in review dumps and VCS markers.
type ReviewMessageRef struct {
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// ReviewMessage is a runtime reviewer message plus its stable reference.
type ReviewMessage struct {
	Ref     ReviewMessageRef `json:"ref"`
	Content string           `json:"content"`
}
