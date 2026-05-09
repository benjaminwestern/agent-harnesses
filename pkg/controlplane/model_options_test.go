package controlplane

import "testing"

func TestMergeModelOptionsPreservesStructuredAndLogprobFields(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}
	merged := MergeModelOptions(ModelOptions{
		ResponseSchema: schema,
		Logprobs:       true,
		TopLogprobs:    5,
	}, ModelOptions{
		ReasoningEffort: "medium",
	})

	if merged.ResponseSchema == nil {
		t.Fatal("ResponseSchema was not preserved")
	}
	if !merged.Logprobs {
		t.Fatal("Logprobs was not preserved")
	}
	if merged.TopLogprobs != 5 {
		t.Fatalf("TopLogprobs = %d, want 5", merged.TopLogprobs)
	}
	if !HasModelOptions(ModelOptions{ResponseSchema: schema}) {
		t.Fatal("HasModelOptions returned false for response schema")
	}
	if !HasModelOptions(ModelOptions{Logprobs: true}) {
		t.Fatal("HasModelOptions returned false for logprobs")
	}
	if !HasModelOptions(ModelOptions{TopLogprobs: 5}) {
		t.Fatal("HasModelOptions returned false for top logprobs")
	}
}

func TestStartSessionRequestNormalizeMirrorsResponseSchema(t *testing.T) {
	schema := map[string]any{"type": "object"}

	topLevel := StartSessionRequest{ResponseSchema: schema}
	topLevel.Normalize()
	if topLevel.ModelOptions.ResponseSchema == nil {
		t.Fatal("top-level ResponseSchema was not copied into ModelOptions")
	}

	optionsOnly := StartSessionRequest{ModelOptions: ModelOptions{ResponseSchema: schema}}
	optionsOnly.Normalize()
	if optionsOnly.ResponseSchema == nil {
		t.Fatal("ModelOptions.ResponseSchema was not copied into top-level ResponseSchema")
	}
}
