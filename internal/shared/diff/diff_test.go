package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
	"github.com/aaudin90/opencode-reviewer/internal/shared/git"
)

const sampleModifiedDiff = `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }
`

const sampleAddedDiff = `diff --git a/new_file.go b/new_file.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/new_file.go
@@ -0,0 +1,3 @@
+package main
+
+func helper() {}
`

const sampleDeletedDiff = `diff --git a/old.go b/old.go
deleted file mode 100644
index abcdefg..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func deprecated() {}
`

const sampleRenamedDiff = `diff --git a/old_name.go b/new_name.go
similarity index 95%
rename from old_name.go
rename to new_name.go
index 1234567..abcdefg 100644
--- a/old_name.go
+++ b/new_name.go
@@ -1,3 +1,3 @@
 package main

-func oldFunc() {}
+func newFunc() {}
`

const sampleMultiFileDiff = sampleModifiedDiff + sampleAddedDiff + sampleDeletedDiff

func TestParseFiles_Modified(t *testing.T) {
	files := parseFiles(sampleModifiedDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Path != "main.go" {
		t.Errorf("expected path main.go, got %q", f.Path)
	}
	if f.ChangeType != Modified {
		t.Errorf("expected modified, got %q", f.ChangeType)
	}
	if f.Added != 3 {
		t.Errorf("expected 3 added lines, got %d", f.Added)
	}
	if f.Deleted != 1 {
		t.Errorf("expected 1 deleted line, got %d", f.Deleted)
	}
	if f.Language != "go" {
		t.Errorf("expected language go, got %q", f.Language)
	}
}

func TestParseFiles_Added(t *testing.T) {
	files := parseFiles(sampleAddedDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.ChangeType != Added {
		t.Errorf("expected added, got %q", f.ChangeType)
	}
	if f.Added != 3 {
		t.Errorf("expected 3 added lines, got %d", f.Added)
	}
	if f.Deleted != 0 {
		t.Errorf("expected 0 deleted lines, got %d", f.Deleted)
	}
}

func TestParseFiles_Deleted(t *testing.T) {
	files := parseFiles(sampleDeletedDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.ChangeType != Deleted {
		t.Errorf("expected deleted, got %q", f.ChangeType)
	}
	if f.Deleted != 3 {
		t.Errorf("expected 3 deleted lines, got %d", f.Deleted)
	}
}

func TestParseFiles_Renamed(t *testing.T) {
	files := parseFiles(sampleRenamedDiff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.ChangeType != Renamed {
		t.Errorf("expected renamed, got %q", f.ChangeType)
	}
	if f.OldPath != "old_name.go" {
		t.Errorf("expected old path old_name.go, got %q", f.OldPath)
	}
	if f.Path != "new_name.go" {
		t.Errorf("expected new path new_name.go, got %q", f.Path)
	}
}

func TestParseFiles_MultiFile(t *testing.T) {
	files := parseFiles(sampleMultiFileDiff)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
}

func TestParseFiles_Empty(t *testing.T) {
	files := parseFiles("")
	if len(files) != 0 {
		t.Fatalf("expected 0 files for empty input, got %d", len(files))
	}
}

func TestIsNoise(t *testing.T) {
	noisy := []string{
		"go.sum",
		"package-lock.json",
		"yarn.lock",
		"api/proto/service.pb.go",
		"assets/logo.png",
		"dist/bundle.min.js",
		".idea/workspace.xml",
		".vscode/settings.json",
		"vendor/github.com/pkg/errors/errors.go",
		"node_modules/lodash/index.js",
		".opencode-review/diff.md",
	}
	for _, p := range noisy {
		if !isNoise(p) {
			t.Errorf("expected %q to be noise", p)
		}
	}

	clean := []string{
		"main.go",
		"internal/diff/diff.go",
		"cmd/app/main.go",
		"README.md",
		"Makefile",
		"src/index.ts",
	}
	for _, p := range clean {
		if isNoise(p) {
			t.Errorf("expected %q to NOT be noise", p)
		}
	}
}

func TestSortFiles(t *testing.T) {
	files := []FileDiff{
		{Path: "README.md", Language: "markdown"},
		{Path: "config.yaml", Language: "yaml"},
		{Path: "Makefile", Language: ""},
		{Path: "main_test.go", Language: "go"},
		{Path: "main.go", Language: "go"},
	}

	sortFiles(files)

	expected := []string{"main.go", "main_test.go", "Makefile", "config.yaml", "README.md"}
	for i, want := range expected {
		if files[i].Path != want {
			t.Errorf("position %d: expected %q, got %q", i, want, files[i].Path)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"App.kt", "kotlin"},
		{"Service.java", "java"},
		{"script.py", "python"},
		{"index.ts", "typescript"},
		{"app.tsx", "typescript"},
		{"lib.rs", "rust"},
		{"config.yaml", "yaml"},
		{"data.json", "json"},
		{"style.css", "css"},
		{"README.md", "markdown"},
		{"config.toml", "toml"},
		{"schema.proto", "protobuf"},
		{"unknown.xyz", ""},
	}

	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	// Diff with headers that stripDiffHeaders removes.
	rawDiff := "diff --git a/main.go b/main.go\nindex 1234567..abcdefg 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+added line\n"
	strippedLen := len(stripDiffHeaders(rawDiff))

	result := &Result{
		Files: []FileDiff{
			{Diff: rawDiff},
		},
	}

	tokens := EstimateTokens(result)
	want := strippedLen / 4
	if tokens != want {
		t.Errorf("EstimateTokens() = %d, want %d (stripped len %d)", tokens, want, strippedLen)
	}

	// Verify that stripped version is shorter than raw (headers removed).
	if strippedLen >= len(rawDiff) {
		t.Errorf("stripped len %d should be less than raw len %d", strippedLen, len(rawDiff))
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	result := &Result{}
	tokens := EstimateTokens(result)
	if tokens != 0 {
		t.Errorf("EstimateTokens() = %d, want 0", tokens)
	}
}

func TestWriteContextFile(t *testing.T) {
	result := &Result{
		Files: []FileDiff{
			{
				Path:       "main.go",
				Language:   "go",
				ChangeType: Modified,
				Added:      5,
				Deleted:    2,
				Diff:       "diff --git a/main.go b/main.go\nindex 1234567..abcdefg 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+added line\n-removed line\n",
			},
			{
				Path:       "new.go",
				Language:   "go",
				ChangeType: Added,
				Added:      10,
				Deleted:    0,
				Diff:       "diff --git a/new.go b/new.go\nnew file mode 100644\nindex 0000000..1234567\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1,1 @@\n+new content\n",
			},
			{
				Path:       "pkg/util.go",
				OldPath:    "internal/util.go",
				Language:   "go",
				ChangeType: Renamed,
				Added:      1,
				Deleted:    1,
				Diff:       "diff --git a/internal/util.go b/pkg/util.go\nsimilarity index 90%\nrename from internal/util.go\nrename to pkg/util.go\nindex 1234567..abcdefg 100644\n--- a/internal/util.go\n+++ b/pkg/util.go\n@@ -1,3 +1,3 @@\n package util\n-func old() {}\n+func new() {}\n",
			},
		},
		FilteredFiles: []string{"go.sum"},
		TotalAdded:    16,
		TotalDeleted:  3,
		DiffStat:      " main.go | 7 ++++---\n 1 file changed",
		CommitLog:     "abc1234 add feature",
		Branch:        "feature",
		BaseBranch:    "main",
	}

	projectDir := t.TempDir()

	path, err := WriteContextFile(result, projectDir)
	if err != nil {
		t.Fatalf("WriteContextFile() error: %v", err)
	}

	expectedPath := filepath.Join(projectDir, config.ReviewDir, "diff.md")
	if path != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading context file: %v", err)
	}

	content := string(data)

	mustContain := []string{
		"# Code Review: feature → main",
		"**Branch:** feature",
		"**Base:** main",
		"**Files changed:** 3",
		"**Lines added:** 16",
		"**Estimated tokens:**",
		"## Commits",
		"abc1234 add feature",
		"## Changed Files",
		"`main.go`",
		"`new.go`",
		"`pkg/util.go`",
		"## Filtered Files (noise)",
		"`go.sum`",
		"## Diffs",
		`<file path="main.go"`,
		`<file path="new.go"`,
		`<file path="pkg/util.go" old_path="internal/util.go"`,
		"</file>",
		"@@ -1,3 +1,4 @@",
	}

	for _, check := range mustContain {
		if !strings.Contains(content, check) {
			t.Errorf("context file missing expected content: %q", check)
		}
	}

	mustNotContain := []string{
		"## Diffstat",
		"diff --git",
		"index 1234567",
		"--- a/main.go",
		"+++ b/main.go",
	}

	for _, check := range mustNotContain {
		if strings.Contains(content, check) {
			t.Errorf("context file should NOT contain: %q", check)
		}
	}

	// Non-renamed files must NOT have old_path attribute.
	if strings.Contains(content, `<file path="main.go" old_path=`) {
		t.Error("modified file should NOT have old_path attribute")
	}
	if strings.Contains(content, `<file path="new.go" old_path=`) {
		t.Error("added file should NOT have old_path attribute")
	}
}

func TestStripDiffHeaders(t *testing.T) {
	input := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
old mode 100644
new mode 100755
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
-	println("hello")
+	fmt.Println("hello")
 }
`
	got := stripDiffHeaders(input)

	mustContain := []string{
		"@@ -1,5 +1,6 @@",
		`+import "fmt"`,
		`-	println("hello")`,
		` package main`,
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("stripDiffHeaders result missing %q", s)
		}
	}

	mustNotContain := []string{
		"diff --git",
		"index 1234567",
		"--- a/main.go",
		"+++ b/main.go",
		"old mode",
		"new mode",
	}
	for _, s := range mustNotContain {
		if strings.Contains(got, s) {
			t.Errorf("stripDiffHeaders result should NOT contain %q", s)
		}
	}
}

func TestStripDiffHeaders_RenameAndNewFile(t *testing.T) {
	input := `diff --git a/old.go b/new.go
similarity index 95%
rename from old.go
rename to new.go
index 1234567..abcdefg 100644
--- a/old.go
+++ b/new.go
@@ -1,3 +1,3 @@
 package main
-func oldFunc() {}
+func newFunc() {}
`
	got := stripDiffHeaders(input)

	if !strings.Contains(got, "@@ -1,3 +1,3 @@") {
		t.Error("missing hunk header")
	}
	for _, s := range []string{"diff --git", "similarity index", "rename from", "rename to"} {
		if strings.Contains(got, s) {
			t.Errorf("should NOT contain %q", s)
		}
	}
}

func TestPrepare(t *testing.T) {
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
	run(workDir, "config", "user.email", "test@test.com")
	run(workDir, "config", "user.name", "Test")

	// Initial commit on main.
	if err := os.WriteFile(filepath.Join(workDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(workDir, "add", "init.txt")
	run(workDir, "commit", "-m", "initial commit")
	run(workDir, "push", "origin", "main")

	// Feature branch with code, tests, noise files.
	run(workDir, "checkout", "-b", "feature")

	if err := os.WriteFile(filepath.Join(workDir, "app.go"), []byte("package main\n\nfunc app() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "app_test.go"), []byte("package main\n\nfunc TestApp() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "go.sum"), []byte("hash1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "config.yaml"), []byte("key: value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run(workDir, "add", ".")
	run(workDir, "commit", "-m", "feature changes")
	run(workDir, "push", "origin", "feature")
	run(workDir, "checkout", "main")

	client := git.NewClient(workDir, "origin")
	if err := client.Fetch(); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	result, err := Prepare(client, "feature", "main")
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	// go.sum should be filtered.
	if len(result.FilteredFiles) != 1 || result.FilteredFiles[0] != "go.sum" {
		t.Errorf("expected go.sum filtered, got %v", result.FilteredFiles)
	}

	// Should have 3 non-noise files.
	if len(result.Files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(result.Files), fileNames(result.Files))
	}

	// Sort order: code first (app.go), then tests (app_test.go), then configs (config.yaml).
	expectedOrder := []string{"app.go", "app_test.go", "config.yaml"}
	for i, want := range expectedOrder {
		if result.Files[i].Path != want {
			t.Errorf("position %d: expected %q, got %q", i, want, result.Files[i].Path)
		}
	}

	if result.TotalAdded == 0 {
		t.Error("expected non-zero TotalAdded")
	}
	if result.Branch != "feature" {
		t.Errorf("expected branch feature, got %q", result.Branch)
	}
	if result.BaseBranch != "main" {
		t.Errorf("expected base branch main, got %q", result.BaseBranch)
	}
	if result.CommitLog == "" {
		t.Error("expected non-empty commit log")
	}
	if result.DiffStat == "" {
		t.Error("expected non-empty diffstat")
	}
}

func fileNames(files []FileDiff) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}
	return names
}
