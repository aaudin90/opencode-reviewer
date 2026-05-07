package runner

type SessionStats struct {
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
	return s
}
