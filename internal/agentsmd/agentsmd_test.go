package agentsmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testContent = `# Режим код-ревью

Проект находится в режиме автоматического код-ревью.

## Запрещённые команды
- gradlew
`

func TestSwap_ContentWritten(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, agentsFile)

	swapper := NewSwapper(dir)

	if _, err := swapper.Swap(testContent); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "Режим код-ревью") {
		t.Error("AGENTS.md should contain provided content")
	}
	if !strings.Contains(got, "gradlew") {
		t.Error("AGENTS.md should contain blocked tools from content")
	}
}

func TestSwap_WithExistingAgentsMD(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, agentsFile)

	if err := os.WriteFile(agentsPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap(testContent)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if len(swapped) != 1 || swapped[0] != agentsPath {
		t.Errorf("Swap returned %v, want [%s]", swapped, agentsPath)
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md after swap: %v", err)
	}

	if !strings.Contains(string(data), "Режим код-ревью") {
		t.Error("AGENTS.md should contain review mode content after swap")
	}
}

func TestSwap_WithoutExistingAgentsMD(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, agentsFile)

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap(testContent)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if len(swapped) != 0 {
		t.Errorf("Swap returned %v, want empty", swapped)
	}

	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatal("AGENTS.md should exist after swap")
	}
}

func TestSwap_WithNestedAgentsMD(t *testing.T) {
	dir := t.TempDir()

	// Create nested directory structure with AGENTS.md at multiple levels.
	sub := filepath.Join(dir, "module", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rootAgents := filepath.Join(dir, agentsFile)
	moduleAgents := filepath.Join(dir, "module", agentsFile)
	subAgents := filepath.Join(sub, agentsFile)

	for _, p := range []struct {
		path    string
		content string
	}{
		{rootAgents, "root original"},
		{moduleAgents, "module original"},
		{subAgents, "sub original"},
	} {
		if err := os.WriteFile(p.path, []byte(p.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	swapper := NewSwapper(dir)

	swapped, err := swapper.Swap(testContent)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	// Verify returned paths cover all three originals.
	sort.Strings(swapped)
	want := []string{rootAgents, moduleAgents, subAgents}
	sort.Strings(want)

	if len(swapped) != len(want) {
		t.Fatalf("Swap returned %d paths, want %d", len(swapped), len(want))
	}
	for i := range want {
		if swapped[i] != want[i] {
			t.Errorf("swapped[%d] = %s, want %s", i, swapped[i], want[i])
		}
	}

	// Root AGENTS.md should be the provided review content.
	data, err := os.ReadFile(rootAgents)
	if err != nil {
		t.Fatalf("read root AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "Режим код-ревью") {
		t.Error("root AGENTS.md should contain review mode content")
	}

	// Nested AGENTS.md files should not exist (they were removed).
	for _, p := range []string{moduleAgents, subAgents} {
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Errorf("%s should not exist after swap", p)
		}
	}
}

func TestSwap_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	swapper := NewSwapper(dir)

	if _, err := swapper.Swap(""); err == nil {
		t.Fatal("Swap with empty content should return error")
	}

	if _, err := swapper.Swap("   \n\t  "); err == nil {
		t.Fatal("Swap with whitespace-only content should return error")
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

	if _, err := swapper.Swap(testContent); err == nil {
		t.Fatal("Swap should return error when directory is read-only")
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

	swapped, err := swapper.Swap(testContent)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if len(swapped) != 0 {
		t.Errorf("Swap returned %v, want empty (heavy dirs should be skipped)", swapped)
	}

	// AGENTS.md inside node_modules should still exist.
	data, err := os.ReadFile(nmAgents)
	if err != nil {
		t.Fatalf("node_modules AGENTS.md should still exist: %v", err)
	}
	if string(data) != "should not be touched" {
		t.Errorf("node_modules AGENTS.md content = %q, want %q", string(data), "should not be touched")
	}
}
