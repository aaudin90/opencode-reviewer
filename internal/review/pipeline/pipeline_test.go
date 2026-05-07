package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
)

func TestRun_FastReviewDoesNotLoadRuntime(t *testing.T) {
	client, workDir := setupPipelineRepo(t)

	dumpPath := filepath.Join(t.TempDir(), "review.json")
	data, err := json.Marshal(finalReviewDump{
		Summary: "summary",
		Verdict: "approve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	p := New(Config{
		GitClient:      client,
		Branch:         "feature",
		BaseBranch:     "main",
		FastReviewPath: dumpPath,
		RuntimeLoader: RuntimeLoaderFunc(func(context.Context) (*RuntimeResources, error) {
			return nil, fmt.Errorf("runtime loader must not be called")
		}),
	})

	review, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if review.Summary != "summary" || review.Verdict != "approve" {
		t.Fatalf("review = %+v, want dump content", review)
	}

	head := runGitOutput(t, workDir, "rev-parse", "--abbrev-ref", "HEAD")
	if strings.TrimSpace(head) != "feature" {
		t.Fatalf("HEAD = %q, want feature", strings.TrimSpace(head))
	}
}

func setupPipelineRepo(t *testing.T) (*git.Client, string) {
	t.Helper()

	tmp := t.TempDir()
	bareDir := filepath.Join(tmp, "remote.git")
	workDir := filepath.Join(tmp, "work")

	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, bareDir, "init", "--bare", "-b", "main")
	runGit(t, tmp, "clone", bareDir, "work")
	runGit(t, workDir, "config", "user.email", "test@test.com")
	runGit(t, workDir, "config", "user.name", "Test")

	writeFile(t, filepath.Join(workDir, "init.txt"), "init\n")
	runGit(t, workDir, "add", "init.txt")
	runGit(t, workDir, "commit", "-m", "initial commit")
	runGit(t, workDir, "push", "origin", "main")

	runGit(t, workDir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(workDir, "feature.txt"), "feature\n")
	runGit(t, workDir, "add", "feature.txt")
	runGit(t, workDir, "commit", "-m", "feature commit")
	runGit(t, workDir, "push", "origin", "feature")
	runGit(t, workDir, "checkout", "main")

	return git.NewClient(workDir, "origin"), workDir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runGitOutput(t, dir, args...)
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
