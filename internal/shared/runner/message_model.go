package runner

import "strings"

type messageModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

func parseModel(model string) *messageModel {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	return &messageModel{
		ProviderID: parts[0],
		ModelID:    parts[1],
	}
}

func formatModel(model *messageModel) string {
	if model == nil || model.ProviderID == "" || model.ModelID == "" {
		return ""
	}
	return model.ProviderID + "/" + model.ModelID
}
