package promptconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnvPaths(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "msg1.md")
	f2 := filepath.Join(dir, "msg2.md")
	if err := os.WriteFile(f1, []byte("content one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("content two"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_MESSAGE_PATHS", f1+","+f2)

	result, err := Load(".", nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	if result[0] != "content one" {
		t.Errorf("result[0] = %q, want %q", result[0], "content one")
	}
	if result[1] != "content two" {
		t.Errorf("result[1] = %q, want %q", result[1], "content two")
	}
}

func TestLoad_TomlInline(t *testing.T) {
	t.Setenv("OR_MESSAGE_PATHS", "")

	inline := []string{"Review for bugs", "Review for security"}
	result, err := Load(".", nil, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	if result[0] != "Review for bugs" {
		t.Errorf("result[0] = %q, want %q", result[0], "Review for bugs")
	}
	if result[1] != "Review for security" {
		t.Errorf("result[1] = %q, want %q", result[1], "Review for security")
	}
}

func TestLoad_TomlPaths(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "msg1.md")
	if err := os.WriteFile(f1, []byte("file content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_MESSAGE_PATHS", "")

	result, err := Load(dir, []string{"msg1.md"}, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0] != "file content" {
		t.Errorf("result[0] = %q, want %q", result[0], "file content")
	}
}

func TestLoad_NothingSet(t *testing.T) {
	t.Setenv("OR_MESSAGE_PATHS", "")

	result, err := Load(".", nil, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if result != nil {
		t.Errorf("got %v, want nil", result)
	}
}

func TestLoad_EnvOverridesTomlInline(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "env.md")
	if err := os.WriteFile(f, []byte("env content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_MESSAGE_PATHS", f)

	inline := []string{"inline content"}
	result, err := Load(".", []string{"/some/path.md"}, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0] != "env content" {
		t.Errorf("result[0] = %q, want %q (env should take priority)", result[0], "env content")
	}
}

func TestLoad_TomlInlineOverridesTomlPaths(t *testing.T) {
	t.Setenv("OR_MESSAGE_PATHS", "")

	inline := []string{"inline msg"}
	result, err := Load(".", []string{"/nonexistent/path.md"}, inline)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0] != "inline msg" {
		t.Errorf("result[0] = %q, want %q (inline should take priority over paths)", result[0], "inline msg")
	}
}

func TestLoadWithOptions_DisablesLegacyEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.md")
	tomlFile := filepath.Join(dir, "toml.md")
	if err := os.WriteFile(envFile, []byte("env content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tomlFile, []byte("toml content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_MESSAGE_PATHS", envFile)

	result, err := LoadWithOptions(dir, []string{"toml.md"}, nil, Options{UseLegacyEnv: false})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if len(result) != 1 || result[0] != "toml content" {
		t.Fatalf("result = %v, want TOML path content when legacy env is disabled", result)
	}
}

func TestLoadWithOptions_LegacyEnvFallback(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.md")
	tomlFile := filepath.Join(dir, "toml.md")
	if err := os.WriteFile(envFile, []byte("env content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tomlFile, []byte("toml content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OR_MESSAGE_PATHS", envFile)

	result, err := LoadWithOptions(dir, []string{"toml.md"}, nil, Options{UseLegacyEnv: true, LegacyEnvFallback: true})
	if err != nil {
		t.Fatalf("LoadWithOptions: %v", err)
	}
	if len(result) != 1 || result[0] != "toml content" {
		t.Fatalf("result = %v, want TOML path content before legacy env fallback", result)
	}
}
