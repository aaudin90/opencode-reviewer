package diff

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/aaudin90/opencode-reviewer/internal/shared/config"
)

var noiseNames = map[string]bool{
	"go.sum":             true,
	"go.mod":             true,
	"package-lock.json":  true,
	"yarn.lock":          true,
	"pnpm-lock.yaml":     true,
	"Podfile.lock":       true,
	"Gemfile.lock":       true,
	"composer.lock":      true,
	"Cargo.lock":         true,
	"poetry.lock":        true,
	"flake.lock":         true,
	"packages.lock.json": true,
}

var noiseSuffixes = []string{
	".pb.go", ".pb.gw.go", "_generated.go",
	".min.js", ".min.css",
	".snap",
	".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".webp",
	".woff", ".woff2", ".ttf", ".eot",
	".pdf", ".zip", ".tar.gz", ".jar",
}

var noisePrefixes = []string{
	config.ReviewDir + "/",
	".idea/",
	".vscode/",
	".gradle/",
	"vendor/",
	"node_modules/",
	"__pycache__/",
}

func isNoise(path string) bool {
	base := filepath.Base(path)
	if noiseNames[base] {
		return true
	}

	for _, s := range noiseSuffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}

	for _, p := range noisePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}

var languageMap = map[string]string{
	".go":     "go",
	".kt":     "kotlin",
	".kts":    "kotlin",
	".java":   "java",
	".py":     "python",
	".js":     "javascript",
	".jsx":    "javascript",
	".ts":     "typescript",
	".tsx":    "typescript",
	".rs":     "rust",
	".rb":     "ruby",
	".c":      "c",
	".h":      "c",
	".cpp":    "cpp",
	".cc":     "cpp",
	".hpp":    "cpp",
	".cs":     "csharp",
	".swift":  "swift",
	".sh":     "bash",
	".bash":   "bash",
	".zsh":    "bash",
	".sql":    "sql",
	".yaml":   "yaml",
	".yml":    "yaml",
	".json":   "json",
	".xml":    "xml",
	".html":   "html",
	".css":    "css",
	".scss":   "scss",
	".md":     "markdown",
	".toml":   "toml",
	".proto":  "protobuf",
	".lua":    "lua",
	".php":    "php",
	".r":      "r",
	".R":      "r",
	".dart":   "dart",
	".scala":  "scala",
	".groovy": "groovy",
}

func detectLanguage(path string) string {
	ext := filepath.Ext(path)
	if lang, ok := languageMap[ext]; ok {
		return lang
	}

	return ""
}

func fileCategory(f FileDiff) int {
	path := f.Path

	if strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, "_test.kt") ||
		strings.HasSuffix(path, "_test.java") ||
		strings.HasSuffix(path, ".test.js") ||
		strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".test.tsx") ||
		strings.HasSuffix(path, ".spec.js") ||
		strings.HasSuffix(path, ".spec.ts") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") {
		return 1
	}

	base := filepath.Base(path)
	switch base {
	case "Makefile", "Dockerfile", "Jenkinsfile",
		"docker-compose.yml", "docker-compose.yaml",
		"build.gradle", "build.gradle.kts", "pom.xml":
		return 2
	}
	if strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, ".ci/") {
		return 2
	}

	switch f.Language {
	case "yaml", "toml", "json", "xml":
		return 3
	}

	if f.Language == "markdown" {
		return 4
	}

	return 0
}

func sortFiles(files []FileDiff) {
	sort.SliceStable(files, func(i, j int) bool {
		return fileCategory(files[i]) < fileCategory(files[j])
	})
}

func changeTypeIcon(ct ChangeType) string {
	switch ct {
	case Added:
		return "✅"
	case Deleted:
		return "❌"
	case Renamed:
		return "📝"
	default:
		return "📄"
	}
}
