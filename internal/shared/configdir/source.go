package configdir

// Source describes where the configuration directory path came from.
type Source string

const (
	SourceNone Source = "none"
	SourceCLI  Source = "cli"
	SourceEnv  Source = "env"
	SourceAuto Source = "auto"
)
