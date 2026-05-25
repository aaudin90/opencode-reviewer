package runner

type SessionStats struct {
	Cost           float64
	Tokens         tokenUsage
	Models         []string
	FallbackModels []string
	ModelCosts     []ModelCost
	messageIDs     map[string]struct{}
	messageCount   int
}

type ModelCost struct {
	Model  string
	Cost   float64
	Tokens tokenUsage
}

func (s SessionStats) Add(other SessionStats) SessionStats {
	s.Cost += other.Cost
	s.Tokens.Input += other.Tokens.Input
	s.Tokens.Output += other.Tokens.Output
	s.Tokens.Reasoning += other.Tokens.Reasoning
	s.Tokens.Cache.Read += other.Tokens.Cache.Read
	s.Tokens.Cache.Write += other.Tokens.Cache.Write
	s.Models = appendUniqueModels(s.Models, other.Models...)
	s.FallbackModels = appendUniqueModels(s.FallbackModels, other.FallbackModels...)
	s.ModelCosts = appendModelCosts(s.ModelCosts, other.ModelCosts...)
	s.messageIDs = appendMessageIDs(s.messageIDs, other.messageIDs)
	s.messageCount += other.messageCount
	return s
}

func appendUniqueModels(base []string, models ...string) []string {
	if len(models) == 0 {
		return base
	}

	out := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(out))
	for _, model := range base {
		if model == "" {
			continue
		}
		seen[model] = struct{}{}
	}

	for _, model := range models {
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}

	return out
}

func appendModelCosts(base []ModelCost, costs ...ModelCost) []ModelCost {
	if len(costs) == 0 {
		return append([]ModelCost(nil), base...)
	}

	out := append([]ModelCost(nil), base...)
	index := make(map[string]int, len(out))
	for i, cost := range out {
		if cost.Model == "" {
			continue
		}
		index[cost.Model] = i
	}

	for _, cost := range costs {
		if cost.Model == "" {
			continue
		}
		if i, ok := index[cost.Model]; ok {
			out[i].Cost += cost.Cost
			out[i].Tokens = addTokenUsage(out[i].Tokens, cost.Tokens)
			continue
		}
		index[cost.Model] = len(out)
		out = append(out, cost)
	}

	return out
}

func addTokenUsage(a, b tokenUsage) tokenUsage {
	a.Input += b.Input
	a.Output += b.Output
	a.Reasoning += b.Reasoning
	a.Cache.Read += b.Cache.Read
	a.Cache.Write += b.Cache.Write
	return a
}

func (s SessionStats) WithMessage(info sessionMessageInfo, requestedModel string) SessionStats {
	model := info.modelString()
	if model == "" {
		model = requestedModel
	}

	s.Cost += info.Cost
	s.Tokens = addTokenUsage(s.Tokens, info.Tokens)
	if model != "" {
		s.Models = appendUniqueModels(s.Models, model)
		s.ModelCosts = appendModelCosts(s.ModelCosts, ModelCost{
			Model:  model,
			Cost:   info.Cost,
			Tokens: info.Tokens,
		})
	}
	if info.ID != "" {
		s.messageIDs = appendMessageIDs(s.messageIDs, map[string]struct{}{info.ID: {}})
	}
	s.messageCount++
	return s
}

func (s SessionStats) HasMessageID(id string) bool {
	if id == "" || s.messageIDs == nil {
		return false
	}
	_, ok := s.messageIDs[id]
	return ok
}

func (s SessionStats) HasMessages() bool {
	return s.messageCount > 0
}

func (s SessionStats) WithFallbackModels(modelChain []string) SessionStats {
	if len(modelChain) < 2 {
		s.FallbackModels = nil
		return s
	}

	fallbacks := make(map[string]struct{}, len(modelChain)-1)
	for _, model := range modelChain[1:] {
		if model == "" {
			continue
		}
		fallbacks[model] = struct{}{}
	}

	var used []string
	for _, model := range s.Models {
		if _, ok := fallbacks[model]; ok {
			used = append(used, model)
		}
	}
	s.FallbackModels = appendUniqueModels(nil, used...)
	return s
}

func appendMessageIDs(base, ids map[string]struct{}) map[string]struct{} {
	if len(base) == 0 && len(ids) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(base)+len(ids))
	for id := range base {
		out[id] = struct{}{}
	}
	for id := range ids {
		out[id] = struct{}{}
	}
	return out
}
