package models

// FinalFinding represents a single deduplicated finding produced by the finalizer agent.
type FinalFinding struct {
	File           string   `json:"file"`
	StartLine      int      `json:"start_line"`
	EndLine        int      `json:"end_line"`
	ExistingCode   string   `json:"existing_code"`
	Confidence     string   `json:"confidence"`
	IssueContent   string   `json:"issue_content"`
	Recommendation string   `json:"recommendation"`
	Sources        []string `json:"sources"`
}
