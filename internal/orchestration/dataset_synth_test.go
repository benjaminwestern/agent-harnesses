package orchestration_test

import (
	"strings"
	"testing"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
)

// Reusing mockEvalFanoutController from dataset_eval_test.go for the fanout generation
// However, since mockEvalFanoutController is in the same package (orchestration_test), we can just use it.
// Oh wait, mockEvalFanoutController is in dataset_eval_test.go. Go test files in the same package share symbols.

// We need a mock for the ledger to provide dataset items, but GenerateSyntheticData accepts a *SQLiteLedgerStore.
// Let's refactor GenerateSyntheticData to accept an interface, or just test the prompt building logic thoroughly.

func TestBuildSyntheticDataPrompt(t *testing.T) {
	basePrompt := "Generate some cool stuff."
	count := 5
	examples := []orchestration.DatasetItemRecord{
		{InputPayload: "Hello", TargetOutput: "World"},
		{InputPayload: "Foo", TargetOutput: "Bar"},
	}

	prompt := orchestration.BuildSyntheticDataPrompt(basePrompt, count, examples)

	if !strings.Contains(prompt, basePrompt) {
		t.Errorf("Expected prompt to contain base prompt")
	}
	if !strings.Contains(prompt, "Example 1:") {
		t.Errorf("Expected prompt to contain few-shot examples")
	}
	if !strings.Contains(prompt, "Input: Hello") {
		t.Errorf("Expected prompt to contain input payload")
	}
	if !strings.Contains(prompt, "Output: World") {
		t.Errorf("Expected prompt to contain target output")
	}
	if !strings.Contains(prompt, "generate 5 new, diverse examples") {
		t.Errorf("Expected prompt to contain generation instructions")
	}
}
