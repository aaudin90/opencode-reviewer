package workspace

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const bunInstallCacheEnv = "BUN_INSTALL_CACHE_DIR"

var opencodeConfigSeedEntries = []string{
	"node_modules",
	"package.json",
	"package-lock.json",
	"bun.lock",
	"bun.lockb",
	"pnpm-lock.yaml",
	"yarn.lock",
}

func seedOpenCodeCaches(dir string) {
	items := []struct {
		name string
		src  string
		dst  string
	}{
		{
			name: "opencode config cache",
			src:  filepath.Join(resolveXDGDir("XDG_CONFIG_HOME", ".config"), opencodeSubdir),
			dst:  filepath.Join(dir, opencodeSubdir),
		},
		{
			name: "opencode cache",
			src:  filepath.Join(resolveXDGDir("XDG_CACHE_HOME", ".cache"), opencodeSubdir),
			dst:  filepath.Join(dir, xdgCacheSubdir, opencodeSubdir),
		},
		{
			name: "bun install cache",
			src:  resolveBunInstallCacheDir(),
			dst:  filepath.Join(dir, xdgCacheSubdir, "bun", "install", "cache"),
		},
	}
	for _, item := range items {
		if err := seedCache(item.src, item.dst, item.name); err != nil {
			slog.Warn("seed workspace cache failed", "cache", item.name, "src", item.src, "dst", item.dst, "error", err)
		}
	}
}

func seedCache(src, dst, name string) error {
	if src == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	if name == "opencode config cache" {
		return copyOpencodeConfigCache(src, dst)
	}
	if name == "opencode cache" {
		return copyDirSkipping(src, dst, map[string]struct{}{
			"snapshot": {},
			"state":    {},
		})
	}
	return copyDir(src, dst)
}

func copyOpencodeConfigCache(src, dst string) error {
	for _, name := range opencodeConfigSeedEntries {
		if err := copyPathIfExists(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyPathIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string) error {
	return copyDirSkipping(src, dst, nil)
}

func copyDirSkipping(src, dst string, skipDirs map[string]struct{}) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && skipDirs != nil {
			if _, skip := skipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(filepath.Clean(dst), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func resolveXDGDir(envName, fallbackSubdir string) string {
	if value := os.Getenv(envName); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, fallbackSubdir)
}

func resolveBunInstallCacheDir() string {
	if value := os.Getenv(bunInstallCacheEnv); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".bun", "install", "cache")
}
