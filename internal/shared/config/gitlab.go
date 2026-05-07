package config

// GitLabConfig holds settings for publishing review results to GitLab MR comments.
type GitLabConfig struct {
	URL           string `toml:"url"`
	Token         string `toml:"token"`
	ProjectID     int    `toml:"project_id"`
	ClearComments bool   `toml:"clear_comments"`
}
