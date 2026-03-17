package agentsmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestSwap_EmptyFilesWritten(t *testing.T) {
	dir := t.TempDir()

	swapper := NewSwapper(dir)

	if _, err := swapper.Swap(); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	for _, name := range []string{agentsFile, claudeFile} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != "" {
			t.Errorf("%s should be empty, got %q", name, string(data))
		}
	}
}

func TestSwap_WithExistingFiles(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, agentsFile)
	claudePath := filepath.Join(dir, claudeFile)

	if err := os.WriteFile(agentsPath, []byte("original agents"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudePath, []byte("original claude"), 0o644); err != nil {
		t.Fatal(err)
	}

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	sort.Strings(swapped)
	want := []string{agentsPath, claudePath}
	sort.Strings(want)

	if len(swapped) != len(want) {
		t.Fatalf("Swap returned %d paths, want %d", len(swapped), len(want))
	}
	for i := range want {
		if swapped[i] != want[i] {
			t.Errorf("swapped[%d] = %s, want %s", i, swapped[i], want[i])
		}
	}

	// Both files should now be empty.
	for _, p := range []string{agentsPath, claudePath} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(data) != "" {
			t.Errorf("%s should be empty after swap, got %q", p, string(data))
		}
	}
}

func TestSwap_WithoutExistingFiles(t *testing.T) {
	dir := t.TempDir()

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if len(swapped) != 0 {
		t.Errorf("Swap returned %v, want empty", swapped)
	}

	// Root files should exist (empty).
	for _, name := range []string{agentsFile, claudeFile} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should exist after swap: %v", name, err)
		}
	}
}

func TestSwap_WithNestedFiles(t *testing.T) {
	dir := t.TempDir()

	// Create nested directory structure with AGENTS.md and CLAUDE.md at multiple levels.
	sub := filepath.Join(dir, "module", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rootAgents := filepath.Join(dir, agentsFile)
	rootClaude := filepath.Join(dir, claudeFile)
	moduleAgents := filepath.Join(dir, "module", agentsFile)
	subClaude := filepath.Join(sub, claudeFile)

	for _, p := range []struct {
		path    string
		content string
	}{
		{rootAgents, "root agents"},
		{rootClaude, "root claude"},
		{moduleAgents, "module agents"},
		{subClaude, "sub claude"},
	} {
		if err := os.WriteFile(p.path, []byte(p.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	// All four files should be in the overwritten list.
	sort.Strings(swapped)
	want := []string{rootAgents, rootClaude, moduleAgents, subClaude}
	sort.Strings(want)

	if len(swapped) != len(want) {
		t.Fatalf("Swap returned %d paths, want %d: %v", len(swapped), len(want), swapped)
	}
	for i := range want {
		if swapped[i] != want[i] {
			t.Errorf("swapped[%d] = %s, want %s", i, swapped[i], want[i])
		}
	}

	// All files should be empty.
	for _, p := range want {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(data) != "" {
			t.Errorf("%s should be empty after swap, got %q", p, string(data))
		}
	}
}

func TestSwap_WriteError(t *testing.T) {
	dir := t.TempDir()

	// Make directory read-only so WriteFile fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
	})

	swapper := NewSwapper(dir)

	if _, err := swapper.Swap(); err == nil {
		t.Fatal("Swap should return error when directory is read-only")
	}
}

func TestSwap_RemovesAgentDirs(t *testing.T) {
	dir := t.TempDir()

	// Create .opencode and .claude directories with some content.
	for _, name := range []string{".opencode", ".claude"} {
		agentDir := filepath.Join(dir, name)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(agentDir, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	swapper := NewSwapper(dir)
	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	// Both directories should be in the returned list.
	removedCount := 0
	for _, p := range swapped {
		for _, name := range []string{".opencode", ".claude"} {
			if p == filepath.Join(dir, name) {
				removedCount++
			}
		}
	}
	if removedCount != 2 {
		t.Errorf("expected 2 agent dirs in swapped list, got %d; swapped = %v", removedCount, swapped)
	}

	// Directories should no longer exist.
	for _, name := range []string{".opencode", ".claude"} {
		agentDir := filepath.Join(dir, name)
		if _, err := os.Stat(agentDir); !os.IsNotExist(err) {
			t.Errorf("%s should be removed after swap", name)
		}
	}
}

func TestSwap_AgentDirsNotPresent(t *testing.T) {
	dir := t.TempDir()

	swapper := NewSwapper(dir)
	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	// No agent dirs to remove, only root files created.
	if len(swapped) != 0 {
		t.Errorf("Swap returned %v, want empty", swapped)
	}
}

func TestSwap_SkipsHeavyDirs(t *testing.T) {
	dir := t.TempDir()

	// Create a heavy directory with AGENTS.md inside.
	nmDir := filepath.Join(dir, "node_modules", "some-package")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}

	nmAgents := filepath.Join(nmDir, agentsFile)
	if err := os.WriteFile(nmAgents, []byte("should not be touched"), 0o644); err != nil {
		t.Fatal(err)
	}

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap()
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if len(swapped) != 0 {
		t.Errorf("Swap returned %v, want empty (heavy dirs should be skipped)", swapped)
	}

	// AGENTS.md inside node_modules should still have original content.
	data, err := os.ReadFile(nmAgents)
	if err != nil {
		t.Fatalf("node_modules AGENTS.md should still exist: %v", err)
	}
	if string(data) != "should not be touched" {
		t.Errorf("node_modules AGENTS.md content = %q, want %q", string(data), "should not be touched")
	}
}
