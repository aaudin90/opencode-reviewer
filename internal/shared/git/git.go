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

// Dir returns the working directory of the git client.
func (c *Client) Dir() string {
	return c.dir
}

// Validate checks that the client directory is a valid git repository.
func (c *Client) Validate() error {
	_, err := c.run("rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("git validate: %w", err)
	}

	return nil
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

// DiffForReview returns the diff with extended context suitable for code review.
func (c *Client) DiffForReview(base, head string) (string, error) {
	out, err := c.run("diff", "-U6", "--no-color", "--find-renames",
		fmt.Sprintf("%s/%s...%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return "", fmt.Errorf("git diff for review: %w", err)
	}

	return out, nil
}

// DiffStat returns the diffstat summary between base and head branches.
func (c *Client) DiffStat(base, head string) (string, error) {
	out, err := c.run("diff", "--stat",
		fmt.Sprintf("%s/%s...%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return "", fmt.Errorf("git diff stat: %w", err)
	}

	return out, nil
}

// Fetch fetches the remote.
func (c *Client) Fetch() error {
	_, err := c.run("fetch", c.remote)
	if err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	return nil
}

// Log returns commit log for head that is not in base (two-dot range).
func (c *Client) Log(base, head string) (string, error) {
	out, err := c.run("log", "--oneline", fmt.Sprintf("%s/%s..%s/%s", c.remote, base, c.remote, head))
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}

	return out, nil
}

// Checkout switches to the specified branch.
func (c *Client) Checkout(branch string) error {
	_, err := c.run("checkout", branch)
	if err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}

	return nil
}

// CheckoutRemote resets a local branch to the current remote branch state.
func (c *Client) CheckoutRemote(branch string) error {
	_, err := c.run("checkout", "-B", branch, fmt.Sprintf("%s/%s", c.remote, branch))
	if err != nil {
		return fmt.Errorf("git checkout remote: %w", err)
	}

	return nil
}

// Clean unstages all changes, resets tracked files and removes untracked files.
func (c *Client) Clean() error {
	if _, err := c.run("reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("git reset: %w", err)
	}

	if _, err := c.run("clean", "-fd"); err != nil {
		return fmt.Errorf("git clean: %w", err)
	}

	return nil
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...) // #nosec G204 -- args are constructed internally, not from user input
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
