package git

import (
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Client performs git operations in a project directory.
type Client struct {
	dir    string
	remote string
}

// NewClient creates a new git client.
func NewClient(dir, remote string) *Client {
	return &Client{dir: dir, remote: remote}
}

// Diff returns the diff between base and head branches.
func (c *Client) Diff(base, head string) (string, error) {
	out, err := c.run("diff", fmt.Sprintf("%s/%s...%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}

	return out, nil
}

// DiffFiles returns the list of changed files between base and head.
func (c *Client) DiffFiles(base, head string) ([]string, error) {
	out, err := c.run("diff", "--name-only", fmt.Sprintf("%s/%s...%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return nil, fmt.Errorf("git diff files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var files []string

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}

	return files, nil
}

// Fetch fetches the remote.
func (c *Client) Fetch() error {
	_, err := c.run("fetch", c.remote)
	if err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	return nil
}

// Log returns commit log between base and head.
func (c *Client) Log(base, head string) (string, error) {
	out, err := c.run("log", "--oneline", fmt.Sprintf("%s/%s...%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}

	return out, nil
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("git exec", "args", args, "dir", c.dir)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
