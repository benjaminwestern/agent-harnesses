package court

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
)

func TestDatasetEvalSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "court.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// 1. Create a Dataset
	ds := orchestration.DatasetRecord{
		ID:               "dataset-1",
		Name:             "Eval Dataset",
		SchemaDefinition: `{"type":"object"}`,
		SourceType:       "synthetic",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := store.ledger.UpsertDataset(ctx, ds); err != nil {
		t.Fatalf("UpsertDataset failed: %v", err)
	}

	// 2. Fetch the Dataset
	fetchedDS, err := store.ledger.GetDataset(ctx, "dataset-1")
	if err != nil {
		t.Fatalf("GetDataset failed: %v", err)
	}
	if fetchedDS.Name != "Eval Dataset" {
		t.Fatalf("Expected dataset name 'Eval Dataset', got '%s'", fetchedDS.Name)
	}

	// 3. Create a Prompt
	prompt := orchestration.PromptRecord{
		ID:        "prompt-1",
		Name:      "Eval Prompt",
		Content:   "Evaluate this.",
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.ledger.UpsertPrompt(ctx, prompt); err != nil {
		t.Fatalf("UpsertPrompt failed: %v", err)
	}

	// 4. Create Dataset Items
	item1 := orchestration.DatasetItemRecord{
		ID:           "item-1",
		DatasetID:    "dataset-1",
		InputPayload: `{"text":"hello"}`,
		TargetOutput: `{"answer":"world"}`,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := store.ledger.UpsertDatasetItem(ctx, item1); err != nil {
		t.Fatalf("UpsertDatasetItem failed: %v", err)
	}

	items, err := store.ledger.ListDatasetItems(ctx, "dataset-1")
	if err != nil {
		t.Fatalf("ListDatasetItems failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "item-1" {
		t.Fatalf("Expected 1 item with ID 'item-1', got %v", items)
	}

	// 5. Create an Evaluation
	eval := orchestration.EvaluationRecord{
		ID:          "eval-1",
		Name:        "Test Eval",
		DatasetID:   "dataset-1",
		PromptID:    "prompt-1",
		TargetModel: "gemini",
		JudgeModel:  "claude",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := store.ledger.UpsertEvaluation(ctx, eval); err != nil {
		t.Fatalf("UpsertEvaluation failed: %v", err)
	}

	fetchedEval, err := store.ledger.GetEvaluation(ctx, "eval-1")
	if err != nil {
		t.Fatalf("GetEvaluation failed: %v", err)
	}
	if fetchedEval.TargetModel != "gemini" {
		t.Fatalf("Expected eval target model 'gemini', got '%s'", fetchedEval.TargetModel)
	}

	// 6. Create an Evaluation Result
	result := orchestration.EvaluationResultRecord{
		ID:            "res-1",
		EvaluationID:  "eval-1",
		DatasetItemID: "item-1",
		Score:         0.95,
		Rationale:     "Looks good",
		Passed:        true,
		LatencyMS:     120,
		CostUSD:       0.001,
		CreatedAt:     time.Now(),
	}
	if err := store.ledger.AddEvaluationResult(ctx, result); err != nil {
		t.Fatalf("AddEvaluationResult failed: %v", err)
	}

	results, err := store.ledger.ListEvaluationResults(ctx, "eval-1")
	if err != nil {
		t.Fatalf("ListEvaluationResults failed: %v", err)
	}
	if len(results) != 1 || results[0].Score != 0.95 {
		t.Fatalf("Expected 1 result with score 0.95, got %v", results)
	}
}
