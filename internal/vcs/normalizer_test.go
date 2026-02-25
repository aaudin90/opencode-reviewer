package vcs

import (
	"testing"

	"github.com/aaudin90/opencode-reviewer/internal/diff"
	"github.com/aaudin90/opencode-reviewer/internal/models"
)

// testDiff is a hand-crafted unified diff for normalizer tests.
// Lines (NewNum): 1=package main, 2="", 3=import "fmt" (+), 4="" (+),
// 5=func main() { (context, was line 3 old), 6=fmt.Println("hello") (+), 7=} (context).
// OldNum lines: 1=package main, 2="", 3=func main() {, 4=println("hello") (-), 5=}.
const testDiff = `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,7 @@
 package main

+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }
`

func makeResult(path, rawDiff string) *diff.Result {
	return &diff.Result{
		Files: []diff.FileDiff{{Path: path, Diff: rawDiff}},
	}
}

func TestNormalize_CorrectLines(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    3,
		EndLine:      3,
		ExistingCode: `import "fmt"`,
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 3 || out[0].EndLine != 3 {
		t.Errorf("expected lines 3-3, got %d-%d", out[0].StartLine, out[0].EndLine)
	}
}

func TestNormalize_ShiftedStartLine(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    10,
		EndLine:      10,
		ExistingCode: `import "fmt"`,
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 3 {
		t.Errorf("expected start_line 3, got %d", out[0].StartLine)
	}
}

func TestNormalize_OldNumInsteadOfNewNum(t *testing.T) {
	// Agent pointed to OldNum for a "+" line — should correct to NewNum.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `	fmt.Println("hello")`,
	}}

	out := n.Normalize(findings)
	// fmt.Println("hello") is an added line with NewNum=6.
	if out[0].StartLine != 6 {
		t.Errorf("expected start_line 6 (NewNum), got %d", out[0].StartLine)
	}
}

func TestNormalize_NewNumInsteadOfOldNum(t *testing.T) {
	// Agent pointed to NewNum for a "-" line — Normalize corrects StartLine to OldNum.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `	println("hello")`,
	}}

	out := n.Normalize(findings)
	// println("hello") is a deleted line with OldNum=4.
	if out[0].StartLine != 4 {
		t.Errorf("expected start_line 4 (OldNum), got %d", out[0].StartLine)
	}
}

func TestNormalizeDiff_OldLineForDeletedLine(t *testing.T) {
	// NormalizeDiff must set OldLine for a deleted "-" line and clear NewLine.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `	println("hello")`,
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	// println("hello") is a deleted line with OldNum=4.
	if out[0].OldLine != 4 {
		t.Errorf("expected OldLine 4 for deleted line, got %d", out[0].OldLine)
	}
	if out[0].NewLine != 0 {
		t.Errorf("expected NewLine 0 for deleted line, got %d", out[0].NewLine)
	}
}

func TestNormalizeDiff_NewLineForAddedLine(t *testing.T) {
	// NormalizeDiff must set NewLine for an added "+" line.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `	fmt.Println("hello")`,
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	// fmt.Println("hello") is an added line with NewNum=6.
	if out[0].NewLine != 6 {
		t.Errorf("expected NewLine 6 for added line, got %d", out[0].NewLine)
	}
	if out[0].OldLine != 0 {
		t.Errorf("expected OldLine 0 for added line, got %d", out[0].OldLine)
	}
}

func TestNormalizeDiff_VerifyAtPosition(t *testing.T) {
	// When StartLine already points to the correct NewNum, NormalizeDiff keeps it.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    3,
		EndLine:      3,
		ExistingCode: `import "fmt"`,
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if out[0].NewLine != 3 {
		t.Errorf("expected NewLine 3, got %d", out[0].NewLine)
	}
	if out[0].OldLine != 0 {
		t.Errorf("expected OldLine 0, got %d", out[0].OldLine)
	}
}

func TestMapFindings_SkipsInvalid(t *testing.T) {
	findings := []models.FinalFinding{
		{File: "", StartLine: 5, IssueContent: "no file"},
		{File: "main.go", StartLine: 0, IssueContent: "no line"},
		{File: "main.go", StartLine: 10, IssueContent: "valid"},
	}
	out := MapFindings(findings)
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if out[0].OldPath != "main.go" || out[0].NewPath != "main.go" || out[0].NewLine != 10 {
		t.Errorf("unexpected DiffFinding: %+v", out[0])
	}
}

func TestNormalize_TrimSpaces(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `  import "fmt"  `,
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 3 {
		t.Errorf("expected start_line 3 after trim, got %d", out[0].StartLine)
	}
}

func TestNormalize_MultiLine(t *testing.T) {
	// Use a diff where 3 added lines are contiguous.
	multiDiff := `diff --git a/svc.go b/svc.go
index 1234567..abcdefg 100644
--- a/svc.go
+++ b/svc.go
@@ -1,3 +1,6 @@
 package main
+func init() {
+	setup()
+}
 func main() {}
`
	n := NewNormalizer(makeResult("svc.go", multiDiff))
	findings := []models.FinalFinding{{
		File:         "svc.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: "func init() {\n\tsetup()\n}",
	}}

	out := n.Normalize(findings)
	// func init() { = NewNum 2, setup() = NewNum 3, } = NewNum 4.
	if out[0].StartLine != 2 {
		t.Errorf("expected start_line 2, got %d", out[0].StartLine)
	}
	if out[0].EndLine != 4 {
		t.Errorf("expected end_line 4, got %d", out[0].EndLine)
	}
}

func TestNormalize_DuplicatePicksClosest(t *testing.T) {
	// Diff with the same line appearing twice at different positions.
	dupDiff := `diff --git a/dup.go b/dup.go
index 1234567..abcdefg 100644
--- a/dup.go
+++ b/dup.go
@@ -1,2 +1,4 @@
 package main
+var x = 1
 var x = 1
+var y = 2
`
	n := NewNormalizer(makeResult("dup.go", dupDiff))
	findings := []models.FinalFinding{{
		File:         "dup.go",
		StartLine:    3,
		EndLine:      3,
		ExistingCode: "var x = 1",
	}}

	out := n.Normalize(findings)
	// "var x = 1" appears at NewNum=2 (added) and NewNum=3 (context).
	// Original start_line=3 → pick NewNum=3 (closest).
	if out[0].StartLine != 3 {
		t.Errorf("expected start_line 3 (closest), got %d", out[0].StartLine)
	}
}

func TestNormalize_NotFound(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    1,
		EndLine:      1,
		ExistingCode: "this code does not exist anywhere",
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 1 || out[0].EndLine != 1 {
		t.Errorf("expected unchanged lines 1-1, got %d-%d", out[0].StartLine, out[0].EndLine)
	}
}

func TestNormalize_FileNotInDiff(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "other.go",
		StartLine:    5,
		EndLine:      5,
		ExistingCode: "some code",
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 5 || out[0].EndLine != 5 {
		t.Errorf("expected unchanged lines 5-5, got %d-%d", out[0].StartLine, out[0].EndLine)
	}
}

func TestNormalize_SuffixMatchFilePath(t *testing.T) {
	// Diff has path "src/main.go", but LLM returns "main.go" (no prefix).
	n := NewNormalizer(makeResult("src/main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    99,
		EndLine:      99,
		ExistingCode: `import "fmt"`,
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 3 {
		t.Errorf("expected start_line 3 via suffix match, got %d", out[0].StartLine)
	}
}

func TestNormalize_EmptyExistingCode(t *testing.T) {
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    1,
		EndLine:      1,
		ExistingCode: "",
	}}

	out := n.Normalize(findings)
	if out[0].StartLine != 1 || out[0].EndLine != 1 {
		t.Errorf("expected unchanged lines 1-1, got %d-%d", out[0].StartLine, out[0].EndLine)
	}
}

func TestNormalizeDiff_InDiffTrue(t *testing.T) {
	// Line 3 (import "fmt") is an added line in testDiff → InDiff must be true.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    3,
		EndLine:      3,
		ExistingCode: `import "fmt"`,
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if !out[0].InDiff {
		t.Errorf("InDiff = false, want true for line present in diff hunk")
	}
}

func TestNormalizeDiff_InDiffFalseOutsideHunk(t *testing.T) {
	// Line 100 is not in any hunk of testDiff; ExistingCode is empty → InDiff must be false.
	n := NewNormalizer(makeResult("main.go", testDiff))
	findings := []models.FinalFinding{{
		File:         "main.go",
		StartLine:    100,
		EndLine:      100,
		ExistingCode: "",
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if out[0].InDiff {
		t.Errorf("InDiff = true, want false for line outside diff hunk")
	}
}

func TestNormalizeDiff_InDiffFalseRenamedNoHunks(t *testing.T) {
	// Pure rename with empty diff — no hunk lines, pathOK=false → InDiff must be false.
	result := &diff.Result{
		Files: []diff.FileDiff{{
			Path:    "new/renamed.go",
			OldPath: "old/renamed.go",
			Diff:    "",
		}},
	}
	n := NewNormalizer(result)

	findings := []models.FinalFinding{{
		File:         "new/renamed.go",
		StartLine:    5,
		EndLine:      5,
		ExistingCode: "",
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if out[0].InDiff {
		t.Errorf("InDiff = true, want false for renamed file with no diff hunks")
	}
}

func TestNormalizeDiff_RenamedFile(t *testing.T) {
	// NormalizeDiff must set OldPath to old file name and NewPath to new file name
	// when the file was renamed in the diff.
	result := &diff.Result{
		Files: []diff.FileDiff{{
			Path:    "new/main.go",
			OldPath: "old/main.go",
			Diff:    testDiff,
		}},
	}
	n := NewNormalizer(result)

	findings := []models.FinalFinding{{
		File:         "new/main.go",
		StartLine:    3,
		EndLine:      3,
		ExistingCode: `import "fmt"`,
	}}

	out := n.NormalizeDiff(MapFindings(findings))
	if len(out) != 1 {
		t.Fatalf("expected 1 DiffFinding, got %d", len(out))
	}
	if out[0].NewPath != "new/main.go" {
		t.Errorf("NewPath = %q, want new/main.go", out[0].NewPath)
	}
	if out[0].OldPath != "old/main.go" {
		t.Errorf("OldPath = %q, want old/main.go", out[0].OldPath)
	}
	if out[0].NewLine != 3 {
		t.Errorf("NewLine = %d, want 3", out[0].NewLine)
	}
}
