package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo creates a bare remote and a working clone with an initial commit on main.
// Returns a Client pointing at the working clone and the clone's directory path.
func setupRepo(t *testing.T) (*Client, string) {
	t.Helper()

	tmp := t.TempDir()
	bareDir := filepath.Join(tmp, "remote.git")
	workDir := filepath.Join(tmp, "work")

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	// Create bare remote.
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(bareDir, "init", "--bare", "-b", "main")

	// Clone into work dir.
	run(tmp, "clone", bareDir, "work")

	// Configure user for commits.
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")

	// Initial commit on main.
	initFile := filepath.Join(workDir, "init.txt")
	if err := os.WriteFile(initFile, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "add", "init.txt")
	run(workDir, "commit", "-m", "initial commit")
	run(workDir, "push", "origin", "main")

	client := NewClient(workDir, "origin")

	return client, workDir
}

func TestFetch(t *testing.T) {
	client, _ := setupRepo(t)

	if err := client.Fetch(); err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
}

func TestCheckout(t *testing.T) {
	client, workDir := setupRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	// Create and push a feature branch.
	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(workDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "feature.txt")
	run("commit", "-m", "feature commit")
	run("push", "origin", "feature")
	run("checkout", "main")

	// Checkout via Client.
	if err := client.Checkout("feature"); err != nil {
		t.Fatalf("Checkout() returned error: %v", err)
	}

	// Verify HEAD is on feature branch.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch != "feature" {
		t.Fatalf("expected branch 'feature', got %q", branch)
	}
}

func TestCheckout_NonExistent(t *testing.T) {
	client, _ := setupRepo(t)

	if err := client.Checkout("nonexistent-branch"); err == nil {
		t.Fatal("Checkout() expected error for nonexistent branch, got nil")
	}
}

func TestClean(t *testing.T) {
	client, workDir := setupRepo(t)

	// Modify tracked file.
	initFile := filepath.Join(workDir, "init.txt")
	if err := os.WriteFile(initFile, []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create untracked file.
	untrackedFile := filepath.Join(workDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := client.Clean(); err != nil {
		t.Fatalf("Clean() returned error: %v", err)
	}

	// Tracked file should be restored.
	data, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("reading init.txt: %v", err)
	}
	if string(data) != "init\n" {
		t.Fatalf("expected tracked file restored to 'init\\n', got %q", string(data))
	}

	// Untracked file should be removed.
	if _, err := os.Stat(untrackedFile); !os.IsNotExist(err) {
		t.Fatal("expected untracked file to be removed")
	}
}

func TestDiff(t *testing.T) {
	client, workDir := setupRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	// Create feature branch with changes.
	run("checkout", "-b", "feat")
	if err := os.WriteFile(filepath.Join(workDir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "new.txt")
	run("commit", "-m", "add new.txt")
	run("push", "origin", "feat")
	run("checkout", "main")

	if err := client.Fetch(); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	diff, err := client.Diff("main", "feat")
	if err != nil {
		t.Fatalf("Diff() returned error: %v", err)
	}
	if diff == "" {
		t.Fatal("Diff() returned empty string, expected non-empty diff")
	}
	if !strings.Contains(diff, "new.txt") {
		t.Fatalf("Diff() output does not mention new.txt: %s", diff)
	}
}

func TestDiffFiles(t *testing.T) {
	client, workDir := setupRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("checkout", "-b", "files-branch")
	if err := os.WriteFile(filepath.Join(workDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt", "b.txt")
	run("commit", "-m", "add a and b")
	run("push", "origin", "files-branch")
	run("checkout", "main")

	if err := client.Fetch(); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	files, err := client.DiffFiles("main", "files-branch")
	if err != nil {
		t.Fatalf("DiffFiles() returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	found := map[string]bool{}
	for _, f := range files {
		found[f] = true
	}
	for _, want := range []string{"a.txt", "b.txt"} {
		if !found[want] {
			t.Errorf("expected file %q in diff, got %v", want, files)
		}
	}
}

func TestLog(t *testing.T) {
	client, workDir := setupRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("checkout", "-b", "log-branch")
	for i := 0; i < 3; i++ {
		fname := filepath.Join(workDir, "log"+string(rune('0'+i))+".txt")
		if err := os.WriteFile(fname, []byte("data\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", "log commit "+string(rune('0'+i)))
	}
	run("push", "origin", "log-branch")
	run("checkout", "main")

	if err := client.Fetch(); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	log, err := client.Log("main", "log-branch")
	if err != nil {
		t.Fatalf("Log() returned error: %v", err)
	}
	if log == "" {
		t.Fatal("Log() returned empty string")
	}

	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d: %s", len(lines), log)
	}
}

func TestInvalidDir(t *testing.T) {
	client := NewClient("/nonexistent/path/that/does/not/exist", "origin")

	if err := client.Validate(); err == nil {
		t.Error("Validate() expected error for invalid dir")
	}
	if err := client.Fetch(); err == nil {
		t.Error("Fetch() expected error for invalid dir")
	}
	if err := client.Checkout("main"); err == nil {
		t.Error("Checkout() expected error for invalid dir")
	}
	if err := client.Clean(); err == nil {
		t.Error("Clean() expected error for invalid dir")
	}
	if _, err := client.Diff("main", "dev"); err == nil {
		t.Error("Diff() expected error for invalid dir")
	}
	if _, err := client.DiffFiles("main", "dev"); err == nil {
		t.Error("DiffFiles() expected error for invalid dir")
	}
	if _, err := client.Log("main", "dev"); err == nil {
		t.Error("Log() expected error for invalid dir")
	}
}

func TestValidate_NotARepo(t *testing.T) {
	tmp := t.TempDir()
	client := NewClient(tmp, "origin")

	if err := client.Validate(); err == nil {
		t.Fatal("Validate() expected error for non-repo directory, got nil")
	}
}
