package commentwarrior

type PipelineConfig struct {
	ProjectDir     string
	Branch         string
	MRIID          int
	DryRun         bool
	MaxDiscussions int
	DiscussionID   string
}
