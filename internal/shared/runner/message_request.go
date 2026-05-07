package runner

type messageRequest struct {
	Parts []messagePart `json:"parts"`
	Agent string        `json:"agent,omitempty"`
	Model *messageModel `json:"model,omitempty"`
}
