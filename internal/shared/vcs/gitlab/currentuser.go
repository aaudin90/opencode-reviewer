package gitlab

// CurrentUser holds basic information about the authenticated GitLab user.
type CurrentUser struct {
	ID int `json:"id"`
}
