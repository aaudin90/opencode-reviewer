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

	got, source, err := Resolve(cliDir, envDir, projectDir, false)
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

	got, source, err := Resolve("", envDir, "", false)
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

	got, source, err := Resolve("", "", projectDir, false)
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

	got, source, err := Resolve("", "", projectDir, false)
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

func TestResolve_AutoDiscoveryFallsBackToCWD(t *testing.T) {
	dir := t.TempDir()
	discovered := filepath.Join(dir, DirectoryName)
	if err := os.Mkdir(discovered, 0o750); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})

	got, source, err := Resolve("", "", "", false)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want, err := filepath.EvalSymlinks(discovered)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if source != SourceAuto {
		t.Fatalf("source = %q, want %q", source, SourceAuto)
	}
}

func TestResolve_DisableAutoDiscovery(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectDir, DirectoryName), 0o750); err != nil {
		t.Fatal(err)
	}

	got, source, err := Resolve("", "", projectDir, true)
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

func TestResolve_DisableAutoDiscoveryDoesNotDisableCLIOrEnv(t *testing.T) {
	cliDir := t.TempDir()
	envDir := t.TempDir()
	projectDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectDir, DirectoryName), 0o750); err != nil {
		t.Fatal(err)
	}

	got, source, err := Resolve(cliDir, envDir, projectDir, true)
	if err != nil {
		t.Fatalf("Resolve CLI: %v", err)
	}
	if got != cliDir || source != SourceCLI {
		t.Fatalf("CLI result = %q/%q, want %q/%q", got, source, cliDir, SourceCLI)
	}

	got, source, err = Resolve("", envDir, projectDir, true)
	if err != nil {
		t.Fatalf("Resolve env: %v", err)
	}
	if got != envDir || source != SourceEnv {
		t.Fatalf("env result = %q/%q, want %q/%q", got, source, envDir, SourceEnv)
	}
}
