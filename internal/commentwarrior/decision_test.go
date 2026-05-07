package commentwarrior

import "testing"

func TestValidateDecisionReplyRequiresBody(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionReply, Confidence: "high"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionRejectsUnsafeFlags(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionNoop, Confidence: "low", WouldModifyCode: true})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDecisionAllowsResolve(t *testing.T) {
	err := ValidateDecision(Decision{Action: ActionResolve, Confidence: "high", Reason: "fixed"})
	if err != nil {
		t.Fatalf("ValidateDecision: %v", err)
	}
}

func TestValidateDecisionAllowsResolveAndUnresolveBody(t *testing.T) {
	tests := []Decision{
		{Action: ActionResolve, Body: "fixed", Confidence: "high", Reason: "fixed"},
		{Action: ActionUnresolve, Body: "still valid", Confidence: "high", Reason: "still valid"},
	}

	for _, tt := range tests {
		if err := ValidateDecision(tt); err != nil {
			t.Fatalf("ValidateDecision(%s): %v", tt.Action, err)
		}
	}
}
