package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Config holds GitLab API connection parameters.
type Config struct {
	URL       string // e.g. "https://gitlab.example.com"
	Token     string // PRIVATE-TOKEN value
	ProjectID int    // numeric project ID
}

// Client is a GitLab REST API client for merge request operations.
type Client struct {
	cfg    Config
	http   *http.Client
	apiURL string
}

// NewClient creates a new GitLab API client.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 30 * time.Second},
		apiURL: cfg.URL + "/api/v4",
	}
}

// FindMergeRequest returns the first open MR matching source and target branches.
func (c *Client) FindMergeRequest(ctx context.Context, source, target string) (*MergeRequestInfo, error) {
	q := url.Values{}
	q.Set("source_branch", source)
	q.Set("target_branch", target)
	q.Set("state", "opened")
	q.Set("per_page", "1")

	path := fmt.Sprintf("/projects/%d/merge_requests", c.cfg.ProjectID)
	body, err := c.doGet(ctx, path, q)
	if err != nil {
		return nil, fmt.Errorf("find merge request: %w", err)
	}

	var mrs []MergeRequestInfo
	if err := json.Unmarshal(body, &mrs); err != nil {
		return nil, fmt.Errorf("decode merge requests: %w", err)
	}
	if len(mrs) == 0 {
		return nil, fmt.Errorf("no open merge request found for %s → %s", source, target)
	}
	return &mrs[0], nil
}

// GetMRDiffRefs returns the diff references (base/head/start SHA) from the
// latest merge request diff version.
func (c *Client) GetMRDiffRefs(ctx context.Context, mrIID int) (*DiffRefs, error) {
	path := fmt.Sprintf("/projects/%d/merge_requests/%d/versions", c.cfg.ProjectID, mrIID)
	body, err := c.doGet(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("get MR diff versions: %w", err)
	}

	var versions []struct {
		BaseSHA  string `json:"base_commit_sha"`
		HeadSHA  string `json:"head_commit_sha"`
		StartSHA string `json:"start_commit_sha"`
	}
	if err := json.Unmarshal(body, &versions); err != nil {
		return nil, fmt.Errorf("decode diff versions: %w", err)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no diff versions found for MR !%d", mrIID)
	}

	v := versions[0]
	return &DiffRefs{
		BaseSHA:  v.BaseSHA,
		HeadSHA:  v.HeadSHA,
		StartSHA: v.StartSHA,
	}, nil
}

// PostMRNote posts a general note (comment) on a merge request.
func (c *Client) PostMRNote(ctx context.Context, mrIID int, body string) error {
	path := fmt.Sprintf("/projects/%d/merge_requests/%d/notes", c.cfg.ProjectID, mrIID)
	payload := map[string]string{"body": body}
	if err := c.doPost(ctx, path, payload); err != nil {
		return fmt.Errorf("post MR note: %w", err)
	}
	return nil
}

// PostDiffNote posts an inline diff note on a merge request at a specific file and line.
// oldPath and newPath are the file paths in the old and new versions of the diff respectively;
// for non-renamed files they are equal.
// Exactly one of oldLine or newLine must be non-zero: use oldLine for deleted lines ("-"),
// newLine for added or context lines.
func (c *Client) PostDiffNote(ctx context.Context, mrIID int, body string, refs *DiffRefs,
	oldPath, newPath string, oldLine, newLine int) error {
	path := fmt.Sprintf("/projects/%d/merge_requests/%d/discussions", c.cfg.ProjectID, mrIID)
	position := map[string]any{
		"position_type":     "text",
		"base_sha":          refs.BaseSHA,
		"head_sha":          refs.HeadSHA,
		"start_sha":         refs.StartSHA,
		"new_path":          newPath,
		"old_path":          oldPath,
		"ignore_whitespace": false,
	}
	if newLine > 0 {
		position["new_line"] = newLine
	}
	if oldLine > 0 {
		position["old_line"] = oldLine
	}
	payload := map[string]any{
		"body":     body,
		"position": position,
	}
	if err := c.doPost(ctx, path, payload); err != nil {
		return fmt.Errorf("post diff note on %s:%d: %w", newPath, oldLine+newLine, err)
	}
	return nil
}

func (c *Client) doGet(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.apiURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.cfg.Token)

	return c.do(req)
}

// GetCurrentUser returns information about the authenticated user.
func (c *Client) GetCurrentUser(ctx context.Context) (*CurrentUser, error) {
	body, err := c.doGet(ctx, "/user", nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	var u CurrentUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("decode current user: %w", err)
	}
	return &u, nil
}

// ClearMRComments deletes open, unanswered discussions on the given MR that were
// created by the current authenticated user. It pages through all discussions and
// removes those that are clearable (single non-system, unresolved note with no replies)
// and belong to the current user. Returns the count of deleted notes.
func (c *Client) ClearMRComments(ctx context.Context, mrIID int) (int, error) {
	me, err := c.GetCurrentUser(ctx)
	if err != nil {
		return 0, fmt.Errorf("get current user for comment clearing: %w", err)
	}

	var deleted int
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		path := fmt.Sprintf("/projects/%d/merge_requests/%d/discussions", c.cfg.ProjectID, mrIID)
		body, err := c.doGet(ctx, path, q)
		if err != nil {
			return deleted, err
		}
		var discussions []mrDiscussion
		if err := json.Unmarshal(body, &discussions); err != nil {
			return deleted, err
		}
		for _, d := range discussions {
			if !d.clearable() {
				continue
			}
			if d.Notes[0].Author.ID != me.ID {
				continue
			}
			noteID := d.Notes[0].ID
			notePath := fmt.Sprintf("/projects/%d/merge_requests/%d/notes/%d",
				c.cfg.ProjectID, mrIID, noteID)
			if err := c.doDelete(ctx, notePath); err != nil {
				slog.Warn("failed to delete MR note", "note_id", noteID, "error", err)
				continue
			}
			deleted++
		}
		if len(discussions) < 100 {
			break
		}
	}
	return deleted, nil
}

func (c *Client) doDelete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.apiURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.cfg.Token)
	_, err = c.do(req)
	return err
}

func (c *Client) doPost(ctx context.Context, path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	_, err = c.do(req)
	return err
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req) // #nosec G704 -- URL built from admin-controlled config, not user input
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		preview := string(body)
		if len(preview) > 1024 {
			preview = preview[:1024]
		}
		return nil, fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, preview)
	}

	return body, nil
}
