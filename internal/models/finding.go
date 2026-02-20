package models

// Finding represents a single code review finding produced by the review agent.
type Finding struct {
	File           string `json:"file"`
	StartLine      int    `json:"start_line"`
	EndLine        int    `json:"end_line"`
	ExistingCode   string `json:"existing_code"`
	Confidence     string `json:"confidence"`
	IssueContent   string `json:"issue_content"`
	Recommendation string `json:"recommendation"`
}
