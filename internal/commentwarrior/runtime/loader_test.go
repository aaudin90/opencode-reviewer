package commentwarriorruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSpecificTextPrefersSpecificConfigDirFile(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(configDir, "comment-warrior", "finding-message.md"), "specific")

	got, err := loadSpecificText(
		configDir,
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		false,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "specific" {
		t.Fatalf("loadSpecificText() = %q, want specific", got)
	}
}

func TestLoadSpecificTextUsesDefaultWhenSpecificConfigDirFileMissing(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(configDir, "comment-warrior", "message.md"), "common")

	got, err := loadSpecificText(
		configDir,
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		false,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "default" {
		t.Fatalf("loadSpecificText() = %q, want default", got)
	}
}

func TestLoadSpecificTextUsesSpecificLegacyEnv(t *testing.T) {
	specific := filepath.Join(t.TempDir(), "specific.md")
	writeRuntimeTestFile(t, specific, "specific env")
	t.Setenv("OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH", specific)

	got, err := loadSpecificText(
		"",
		"comment-warrior/finding-message.md",
		"OR_COMMENT_WARRIOR_FINDING_MESSAGE_PATH",
		"default",
		true,
	)
	if err != nil {
		t.Fatalf("loadSpecificText: %v", err)
	}
	if got != "specific env" {
		t.Fatalf("loadSpecificText() = %q, want specific env", got)
	}
}

func writeRuntimeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
