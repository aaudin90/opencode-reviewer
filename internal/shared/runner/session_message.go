package runner

type sessionMessage struct {
	Info sessionMessageInfo `json:"info"`
}

type sessionMessageInfo struct {
	ID         string        `json:"id"`
	Role       string        `json:"role"`
	Cost       float64       `json:"cost"`
	Tokens     tokenUsage    `json:"tokens"`
	ProviderID string        `json:"providerID"`
	ModelID    string        `json:"modelID"`
	Model      *messageModel `json:"model"`
}

func (i sessionMessageInfo) modelString() string {
	if i.ProviderID != "" && i.ModelID != "" {
		return i.ProviderID + "/" + i.ModelID
	}
	return formatModel(i.Model)
}
