package runner

type messageResponse struct {
	Info  messageInfo   `json:"info"`
	Parts []messagePart `json:"parts"`
}
