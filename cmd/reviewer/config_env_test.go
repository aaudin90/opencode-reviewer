package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveDeprecatedConfigEnv(t *testing.T) {
	for _, key := range deprecatedConfigEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("OR_PROVIDER_CONFIG", `{"provider":{}}`)
	t.Setenv("OR_MESSAGE_PATHS", "/tmp/message.md")

	got := activeDeprecatedConfigEnv()
	want := []string{"OR_PROVIDER_CONFIG", "OR_MESSAGE_PATHS"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("activeDeprecatedConfigEnv = %v, want %v", got, want)
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want bool
	}{
		{name: "true", val: "true", want: true},
		{name: "one", val: "1", want: true},
		{name: "case and spaces", val: " TRUE ", want: true},
		{name: "false", val: "false", want: false},
		{name: "empty", val: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tt.val)
			if got := envBool("TEST_BOOL"); got != tt.want {
				t.Fatalf("envBool = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadToolOverrides_MissingDir(t *testing.T) {
	got, err := loadToolOverrides(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("loadToolOverrides: %v", err)
	}
	if got != nil {
		t.Fatalf("overrides = %v, want nil", got)
	}
}

func TestLoadToolOverrides_ReadDirError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadToolOverrides(path)
	if err == nil {
		t.Fatal("expected read dir error")
	}
}

func TestLoadToolOverrides_LoadsOnlyTypeScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "submit_review.ts"), []byte("override"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadToolOverrides(dir)
	if err != nil {
		t.Fatalf("loadToolOverrides: %v", err)
	}
	if len(got) != 1 || string(got["submit_review.ts"]) != "override" {
		t.Fatalf("overrides = %v, want submit_review.ts only", got)
	}
}

func TestLoadConfigDirToolOverrides_LoadsPhaseTools(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "reviewer", "tools")
	if err := os.MkdirAll(toolsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "submit_review.ts"), []byte("override"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "notes.md"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadConfigDirToolOverrides(dir, "reviewer")
	if err != nil {
		t.Fatalf("loadConfigDirToolOverrides: %v", err)
	}
	if len(got) != 1 || string(got["submit_review.ts"]) != "override" {
		t.Fatalf("overrides = %v, want submit_review.ts only", got)
	}
}

func TestLoadConfigDirToolOverrides_MissingPhaseToolsDir(t *testing.T) {
	got, err := loadConfigDirToolOverrides(t.TempDir(), "finalizer")
	if err != nil {
		t.Fatalf("loadConfigDirToolOverrides: %v", err)
	}
	if got != nil {
		t.Fatalf("overrides = %v, want nil", got)
	}
}
