package reviewruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/review/pipeline"
	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
)

func TestLoaderLoadsConfigDirAfterCheckout(t *testing.T) {
	client, workDir := setupRepo(t)
	clearConfigEnv(t)

	runGit(t, workDir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(workDir, ".opencodereview", "reviewer", "messages", "01.md"), "branch message")
	runGit(t, workDir, "add", ".opencodereview")
	runGit(t, workDir, "commit", "-m", "add review config")
	runGit(t, workDir, "push", "origin", "feature")
	runGit(t, workDir, "checkout", "main")

	if err := git.PrepareRepository(client, "feature"); err != nil {
		t.Fatalf("PrepareRepository: %v", err)
	}

	resources := loadRuntime(t, workDir)
	if len(resources.Messages) != 1 || resources.Messages[0].Content != "branch message" {
		t.Fatalf("messages = %v, want branch message", resources.Messages)
	}
	if got := resources.Messages[0].Ref.Path; got != ".opencodereview/reviewer/messages/01.md" {
		t.Fatalf("message ref path = %q, want relative config-dir path", got)
	}
}

func TestLoaderUsesConfigDirVersionFromCheckedOutBranch(t *testing.T) {
	client, workDir := setupRepo(t)
	clearConfigEnv(t)

	writeFile(t, filepath.Join(workDir, ".opencodereview", "reviewer", "messages", "01.md"), "main message")
	runGit(t, workDir, "add", ".opencodereview")
	runGit(t, workDir, "commit", "-m", "add main review config")
	runGit(t, workDir, "push", "origin", "main")

	runGit(t, workDir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(workDir, ".opencodereview", "reviewer", "messages", "01.md"), "feature message")
	runGit(t, workDir, "add", ".opencodereview")
	runGit(t, workDir, "commit", "-m", "change review config")
	runGit(t, workDir, "push", "origin", "feature")
	runGit(t, workDir, "checkout", "main")

	if err := git.PrepareRepository(client, "feature"); err != nil {
		t.Fatalf("PrepareRepository: %v", err)
	}

	resources := loadRuntime(t, workDir)
	if len(resources.Messages) != 1 || resources.Messages[0].Content != "feature message" {
		t.Fatalf("messages = %v, want feature message", resources.Messages)
	}
}

func TestLoaderUsesDeprecatedMessageEnvWhenConfigDirInactive(t *testing.T) {
	client, workDir := setupRepo(t)
	clearConfigEnv(t)

	envMessage := filepath.Join(t.TempDir(), "env-message.md")
	writeFile(t, envMessage, "env message")
	t.Setenv("OR_MESSAGE_PATHS", envMessage)

	if err := git.PrepareRepository(client, "main"); err != nil {
		t.Fatalf("PrepareRepository: %v", err)
	}

	resources := loadRuntime(t, workDir)
	if len(resources.Messages) != 1 || resources.Messages[0].Content != "env message" {
		t.Fatalf("messages = %v, want env message", resources.Messages)
	}
}

func TestLoaderIgnoresDeprecatedMessageEnvWhenConfigDirActive(t *testing.T) {
	client, workDir := setupRepo(t)
	clearConfigEnv(t)

	envMessage := filepath.Join(t.TempDir(), "env-message.md")
	writeFile(t, envMessage, "env message")
	t.Setenv("OR_MESSAGE_PATHS", envMessage)

	writeFile(t, filepath.Join(workDir, ".opencodereview", "reviewer", "messages", "01.md"), "config-dir message")
	runGit(t, workDir, "add", ".opencodereview")
	runGit(t, workDir, "commit", "-m", "add review config")
	runGit(t, workDir, "push", "origin", "main")

	if err := git.PrepareRepository(client, "main"); err != nil {
		t.Fatalf("PrepareRepository: %v", err)
	}

	resources := loadRuntime(t, workDir)
	if len(resources.Messages) != 1 || resources.Messages[0].Content != "config-dir message" {
		t.Fatalf("messages = %v, want config-dir message", resources.Messages)
	}
}

func setupRepo(t *testing.T) (*git.Client, string) {
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

	return git.NewClient(workDir, "origin"), workDir
}

func loadRuntime(t *testing.T, workDir string) *pipeline.RuntimeResources {
	t.Helper()
	cacheHome := t.TempDir()
	t.Setenv("HOME", cacheHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(cacheHome, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cacheHome, ".cache"))
	t.Setenv("BUN_INSTALL_CACHE_DIR", filepath.Join(cacheHome, ".bun", "install", "cache"))

	loader := New(Config{
		AppConfig: &config.Config{
			ProjectDir: workDir,
			OpenCode: config.OpenCodeConfig{
				Endpoint: "http://127.0.0.1:1",
			},
		},
		ConfigBaseDir: ".",
	})

	resources, err := loader.LoadRuntime(context.Background())
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	t.Cleanup(func() {
		if resources.Cleanup != nil {
			_ = resources.Cleanup()
		}
	})
	return resources
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OR_CONFIG_DIR",
		"OR_PROVIDER_CONFIG_PATH",
		"OR_PROVIDER_CONFIG",
		"OR_AGENT_PROMPT_PATH",
		"OR_MESSAGE_PATHS",
		"OR_FINALIZER_PROMPT_PATH",
		"OR_FINALIZER_MESSAGE_PATH",
		"OR_REVIEW_SUB_AGENT_PROMPT_PATHS",
		"OR_FINALIZER_SUB_AGENT_PROMPT_PATHS",
	} {
		t.Setenv(key, "")
	}
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
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
