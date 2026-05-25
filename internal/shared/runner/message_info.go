package runner

type messageInfo struct {
	ID         string        `json:"id"`
	Cost       float64       `json:"cost"`
	Tokens     tokenUsage    `json:"tokens"`
	ProviderID string        `json:"providerID"`
	ModelID    string        `json:"modelID"`
	Model      *messageModel `json:"model"`
}

func (i messageInfo) modelString() string {
	if i.ProviderID != "" && i.ModelID != "" {
		return i.ProviderID + "/" + i.ModelID
	}
	return formatModel(i.Model)
}
