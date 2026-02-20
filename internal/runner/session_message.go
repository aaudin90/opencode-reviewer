package runner

type sessionMessage struct {
	Info sessionMessageInfo `json:"info"`
}

type sessionMessageInfo struct {
	Role   string     `json:"role"`
	Cost   float64    `json:"cost"`
	Tokens tokenUsage `json:"tokens"`
}
