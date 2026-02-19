package config

type OutputConfig struct {
	FilePath         string `toml:"file_path"`
	FormatProjectDir string `toml:"format_project_dir"`
}
