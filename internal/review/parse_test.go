package review

import (
	"testing"
)

func TestParse_ValidJSON(t *testing.T) {
	raw := `[{"file":"main.go","start_line":10,"end_line":10,"symbol":"main","existing_code":"x := 1","severity":"possible bug","confidence":"high","issue_content":"unused","recommendation":"remove it"}]`

	result := Parse(raw)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("len(Findings) = %d, want 1", len(result.Findings))
	}
	f := result.Findings[0]
	if f.File != "main.go" {
		t.Errorf("File = %q, want %q", f.File, "main.go")
	}
	if f.StartLine != 10 {
		t.Errorf("StartLine = %d, want 10", f.StartLine)
	}
	if result.Raw != raw {
		t.Errorf("Raw not preserved")
	}
}

func TestParse_EmptyArray(t *testing.T) {
	result := Parse("[]")

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if len(result.Findings) != 0 {
		t.Errorf("len(Findings) = %d, want 0", len(result.Findings))
	}
}

func TestParse_JSONWrappedInCodeFence(t *testing.T) {
	raw := "```json\n[{\"file\":\"a.go\",\"start_line\":1,\"end_line\":1,\"symbol\":\"f\",\"existing_code\":\"x\",\"severity\":\"security\",\"confidence\":\"high\",\"issue_content\":\"bad\",\"recommendation\":\"fix\"}]\n```"

	result := Parse(raw)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil (extraction should succeed)", result.ParseErr)
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}

func TestParse_JSONWithSurroundingProse(t *testing.T) {
	raw := `Here are the findings:
[{"file":"b.go","start_line":5,"end_line":5,"symbol":"g","existing_code":"y","severity":"possible bug","confidence":"medium","issue_content":"oops","recommendation":"fix it"}]
That's all.`

	result := Parse(raw)

	if result.ParseErr != nil {
		t.Fatalf("ParseErr = %v, want nil", result.ParseErr)
	}
	if len(result.Findings) != 1 {
		t.Errorf("len(Findings) = %d, want 1", len(result.Findings))
	}
}

func TestParse_InvalidJSON_SetsParseErr(t *testing.T) {
	raw := "not json at all"

	result := Parse(raw)

	if result.ParseErr == nil {
		t.Error("ParseErr = nil, want non-nil for invalid input")
	}
	if result.Raw != raw {
		t.Error("Raw not preserved on parse failure")
	}
	if result.Findings != nil {
		t.Errorf("Findings = %v, want nil on parse failure", result.Findings)
	}
}

func TestParse_RawAlwaysPreserved(t *testing.T) {
	raw := `[{"file":"c.go","start_line":2,"end_line":3,"symbol":"h","existing_code":"z","severity":"enhancement","confidence":"low","issue_content":"minor","recommendation":"consider"}]`

	result := Parse(raw)

	if result.Raw != raw {
		t.Error("Raw not preserved")
	}
}
