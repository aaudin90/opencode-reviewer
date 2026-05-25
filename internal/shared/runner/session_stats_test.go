package runner

import "testing"

func TestSessionStats_Add_ZeroValues(t *testing.T) {
	a := SessionStats{}
	b := SessionStats{}
	result := a.Add(b)
	if result.Cost != 0 {
		t.Errorf("Cost = %v, want 0", result.Cost)
	}
	if result.Tokens.Input != 0 {
		t.Errorf("Tokens.Input = %d, want 0", result.Tokens.Input)
	}
	if result.Tokens.Output != 0 {
		t.Errorf("Tokens.Output = %d, want 0", result.Tokens.Output)
	}
	if result.Tokens.Reasoning != 0 {
		t.Errorf("Tokens.Reasoning = %d, want 0", result.Tokens.Reasoning)
	}
	if result.Tokens.Cache.Read != 0 {
		t.Errorf("Tokens.Cache.Read = %d, want 0", result.Tokens.Cache.Read)
	}
	if result.Tokens.Cache.Write != 0 {
		t.Errorf("Tokens.Cache.Write = %d, want 0", result.Tokens.Cache.Write)
	}
	if len(result.Models) != 0 {
		t.Errorf("len(Models) = %d, want 0", len(result.Models))
	}
	if len(result.FallbackModels) != 0 {
		t.Errorf("len(FallbackModels) = %d, want 0", len(result.FallbackModels))
	}
	if len(result.ModelCosts) != 0 {
		t.Errorf("len(ModelCosts) = %d, want 0", len(result.ModelCosts))
	}
}

func TestSessionStats_Add_AccumulatesAllFields(t *testing.T) {
	a := SessionStats{
		Cost: 1.5,
		Tokens: tokenUsage{
			Input:     100,
			Output:    200,
			Reasoning: 50,
			Cache:     tokenCache{Read: 10, Write: 20},
		},
		Models:         []string{"p/primary", "p/fallback"},
		FallbackModels: []string{"p/fallback"},
		ModelCosts: []ModelCost{
			{Model: "p/primary", Cost: 1.0, Tokens: tokenUsage{Input: 70, Output: 100}},
			{Model: "p/fallback", Cost: 0.5, Tokens: tokenUsage{Input: 30, Output: 100}},
		},
	}
	b := SessionStats{
		Cost: 0.5,
		Tokens: tokenUsage{
			Input:     50,
			Output:    75,
			Reasoning: 25,
			Cache:     tokenCache{Read: 5, Write: 10},
		},
		Models:         []string{"p/fallback", "p/secondary"},
		FallbackModels: []string{"p/fallback", "p/secondary"},
		ModelCosts: []ModelCost{
			{Model: "p/fallback", Cost: 0.25, Tokens: tokenUsage{Input: 25, Output: 25}},
			{Model: "p/secondary", Cost: 0.25, Tokens: tokenUsage{Input: 25, Output: 50}},
		},
	}
	result := a.Add(b)
	if result.Cost != 2.0 {
		t.Errorf("Cost = %v, want 2.0", result.Cost)
	}
	if result.Tokens.Input != 150 {
		t.Errorf("Tokens.Input = %d, want 150", result.Tokens.Input)
	}
	if result.Tokens.Output != 275 {
		t.Errorf("Tokens.Output = %d, want 275", result.Tokens.Output)
	}
	if result.Tokens.Reasoning != 75 {
		t.Errorf("Tokens.Reasoning = %d, want 75", result.Tokens.Reasoning)
	}
	if result.Tokens.Cache.Read != 15 {
		t.Errorf("Tokens.Cache.Read = %d, want 15", result.Tokens.Cache.Read)
	}
	if result.Tokens.Cache.Write != 30 {
		t.Errorf("Tokens.Cache.Write = %d, want 30", result.Tokens.Cache.Write)
	}
	assertModelStrings(t, result.Models, []string{"p/primary", "p/fallback", "p/secondary"})
	assertModelStrings(t, result.FallbackModels, []string{"p/fallback", "p/secondary"})
	assertModelCosts(t, result.ModelCosts, []ModelCost{
		{Model: "p/primary", Cost: 1.0, Tokens: tokenUsage{Input: 70, Output: 100}},
		{Model: "p/fallback", Cost: 0.75, Tokens: tokenUsage{Input: 55, Output: 125}},
		{Model: "p/secondary", Cost: 0.25, Tokens: tokenUsage{Input: 25, Output: 50}},
	})
}

func TestSessionStats_Add_IsNotMutating(t *testing.T) {
	a := SessionStats{
		Cost:       1.0,
		Tokens:     tokenUsage{Input: 100, Output: 200},
		Models:     []string{"p/primary"},
		ModelCosts: []ModelCost{{Model: "p/primary", Cost: 1.0, Tokens: tokenUsage{Input: 100}}},
	}
	b := SessionStats{
		Cost:   0.5,
		Tokens: tokenUsage{Input: 50, Output: 75},
	}
	_ = a.Add(b)
	if a.Cost != 1.0 {
		t.Errorf("a.Cost = %v, want 1.0 (Add should not mutate receiver)", a.Cost)
	}
	if a.Tokens.Input != 100 {
		t.Errorf("a.Tokens.Input = %d, want 100 (Add should not mutate receiver)", a.Tokens.Input)
	}
	if a.Tokens.Output != 200 {
		t.Errorf("a.Tokens.Output = %d, want 200 (Add should not mutate receiver)", a.Tokens.Output)
	}
	assertModelStrings(t, a.Models, []string{"p/primary"})
	assertModelCosts(t, a.ModelCosts, []ModelCost{{Model: "p/primary", Cost: 1.0, Tokens: tokenUsage{Input: 100}}})
}

func TestSessionStats_Add_Chained(t *testing.T) {
	s1 := SessionStats{Cost: 1.0}
	s2 := SessionStats{Cost: 1.0}
	s3 := SessionStats{Cost: 1.0}
	result := s1.Add(s2).Add(s3)
	if result.Cost != 3.0 {
		t.Errorf("Cost = %v, want 3.0", result.Cost)
	}
}

func TestSessionStats_WithFallbackModels(t *testing.T) {
	stats := SessionStats{Models: []string{"p/fallback", "p/primary", "p/secondary"}}
	got := stats.WithFallbackModels([]string{"p/primary", "p/fallback", "p/secondary"})
	assertModelStrings(t, got.FallbackModels, []string{"p/fallback", "p/secondary"})
}

func assertModelStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(Models) = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Models[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func assertModelCosts(t *testing.T, got, want []ModelCost) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(ModelCosts) = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Model != want[i].Model || got[i].Cost != want[i].Cost || got[i].Tokens.Input != want[i].Tokens.Input || got[i].Tokens.Output != want[i].Tokens.Output {
			t.Fatalf("ModelCosts[%d] = %#v, want %#v; got %#v", i, got[i], want[i], got)
		}
	}
}
