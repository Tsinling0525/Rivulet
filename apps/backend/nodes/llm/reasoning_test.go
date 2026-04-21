package llm

import "testing"

func TestSplitReasoningTextUsesBullets(t *testing.T) {
	steps := SplitReasoningText("- inspect input\n- choose route\n- format output")
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d: %#v", len(steps), steps)
	}
	if steps[0] != "inspect input" || steps[2] != "format output" {
		t.Fatalf("unexpected steps: %#v", steps)
	}
}

func TestExtractThinkBlocks(t *testing.T) {
	got := ExtractThinkBlocks("<think>check constraints</think>\nfinal answer\n<think>verify result</think>")
	want := "check constraints\n\nverify result"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
