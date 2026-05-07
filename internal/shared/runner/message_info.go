package runner

type messageInfo struct {
	ID     string     `json:"id"`
	Cost   float64    `json:"cost"`
	Tokens tokenUsage `json:"tokens"`
}
