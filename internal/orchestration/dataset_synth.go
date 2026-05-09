package orchestration

import (
	"context"
	"fmt"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type SyntheticGenerationOptions struct {
	DatasetID    string
	BasePrompt   string
	Schema       map[string]any
	Count        int
	Targets      []FanoutTarget
	KeepSessions bool
}

// BuildSyntheticDataPrompt formats a few-shot prompt from existing dataset items.
func BuildSyntheticDataPrompt(basePrompt string, count int, examples []DatasetItemRecord) string {
	var promptBuilder strings.Builder
	promptBuilder.WriteString(basePrompt)
	promptBuilder.WriteString("\n\n")

	if len(examples) > 0 {
		promptBuilder.WriteString("Here are some examples of the desired output format and style:\n\n")
		for i, ex := range examples {
			fmt.Fprintf(&promptBuilder, "Example %d:\n", i+1)
			fmt.Fprintf(&promptBuilder, "Input: %s\n", ex.InputPayload)
			fmt.Fprintf(&promptBuilder, "Output: %s\n\n", ex.TargetOutput)
		}
	}

	fmt.Fprintf(&promptBuilder, "Please generate %d new, diverse examples following the same schema and format. Output strictly valid JSON matching the schema.", count)
	return promptBuilder.String()
}

// GenerateSyntheticData queries the dataset_items table for a subset of human-approved rows,
// formats these rows into a Few-Shot context block, and injects the block into the FanoutOptions.Prompt
// before orchestrating a bulk generation run.
func GenerateSyntheticData(ctx context.Context, ledger *SQLiteLedgerStore, controller FanoutController, opts SyntheticGenerationOptions) (FanoutResult, error) {
	items, err := ledger.ListDatasetItems(ctx, opts.DatasetID)
	if err != nil {
		return FanoutResult{}, fmt.Errorf("list dataset items: %w", err)
	}

	var examples []DatasetItemRecord
	for _, item := range items {
		// Only select approved/active items for few-shot
		if item.Status == "approved" || item.Status == "active" {
			examples = append(examples, item)
			if len(examples) >= 5 { // Limit to 5 few-shot examples
				break
			}
		}
	}

	fanoutOpts := FanoutOptions{
		Prompt:       BuildSyntheticDataPrompt(opts.BasePrompt, opts.Count, examples),
		Targets:      opts.Targets,
		KeepSessions: opts.KeepSessions,
		ModelOptions: api.ModelOptions{
			ResponseSchema: opts.Schema,
		},
	}

	return RunFanout(ctx, controller, fanoutOpts)
}
