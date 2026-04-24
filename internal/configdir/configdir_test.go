package configdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_CLIHasPriority(t *testing.T) {
	cliDir := t.TempDir()
	envDir := t.TempDir()
	projectDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectDir, DirectoryName), 0o750); err != nil {
		t.Fatal(err)
	}

	got, source, err := Resolve(cliDir, envDir, projectDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != cliDir {
		t.Fatalf("path = %q, want %q", got, cliDir)
	}
	if source != SourceCLI {
		t.Fatalf("source = %q, want %q", source, SourceCLI)
	}
}

func TestResolve_EnvFallback(t *testing.T) {
	envDir := t.TempDir()

	got, source, err := Resolve("", envDir, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != envDir {
		t.Fatalf("path = %q, want %q", got, envDir)
	}
	if source != SourceEnv {
		t.Fatalf("source = %q, want %q", source, SourceEnv)
	}
}

func TestResolve_AutoDiscovery(t *testing.T) {
	projectDir := t.TempDir()
	discovered := filepath.Join(projectDir, DirectoryName)
	if err := os.Mkdir(discovered, 0o750); err != nil {
		t.Fatal(err)
	}

	got, source, err := Resolve("", "", projectDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != discovered {
		t.Fatalf("path = %q, want %q", got, discovered)
	}
	if source != SourceAuto {
		t.Fatalf("source = %q, want %q", source, SourceAuto)
	}
}

func TestResolve_NoneWhenNotFound(t *testing.T) {
	projectDir := t.TempDir()

	got, source, err := Resolve("", "", projectDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Fatalf("path = %q, want empty", got)
	}
	if source != SourceNone {
		t.Fatalf("source = %q, want %q", source, SourceNone)
	}
}
