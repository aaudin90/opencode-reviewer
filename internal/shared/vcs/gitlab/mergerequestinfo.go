package gitlab

// MergeRequestInfo holds basic information about a GitLab merge request.
type MergeRequestInfo struct {
	IID   int    `json:"iid"`
	Title string `json:"title"`
	State string `json:"state"`
	SHA   string `json:"sha"`
}
