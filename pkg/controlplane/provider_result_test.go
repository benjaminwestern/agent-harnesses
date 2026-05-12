package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type wrappedProviderResult struct {
	result ProviderResultMetadata
}

func (w wrappedProviderResult) Error() string {
	return "wrapped provider result"
}

func (w wrappedProviderResult) ProviderResult() (ProviderResultMetadata, bool) {
	return w.result, true
}

func TestProviderResultErrorCarriesMetadata(t *testing.T) {
	result := ProviderResultMetadata{Provider: "ollama", Model: "llama3.2", OutputKind: "empty_final_content"}
	err := fmt.Errorf("outer: %w", NewProviderResultError("empty output", result, nil))

	var carrier ProviderResultCarrier
	if !errors.As(err, &carrier) {
		t.Fatalf("errors.As did not find ProviderResultCarrier in %T", err)
	}
	got, ok := carrier.ProviderResult()
	if !ok || got.Provider != "ollama" || got.Model != "llama3.2" || got.OutputKind != "empty_final_content" {
		t.Fatalf("ProviderResult() = %+v, %v", got, ok)
	}
}

func TestResultAggregatorAddsMergesAndSerializesSummary(t *testing.T) {
	var aggregator ResultAggregator
	aggregator.Add(ProviderResultMetadata{
		Provider:     "ollama",
		Model:        "llama3.2",
		RequestID:    "req-1",
		StatusCode:   200,
		OutputKind:   "text",
		FinishReason: "stop",
	})
	if ok := aggregator.AddError(fmt.Errorf("outer: %w", wrappedProviderResult{result: ProviderResultMetadata{
		Provider:   "ollama",
		Model:      "llama3.2",
		RequestID:  "req-2",
		StatusCode: 503,
		OutputKind: "provider_error",
		Error:      &ProviderError{Kind: "api", Message: "busy"},
	}})); !ok {
		t.Fatal("AddError returned false, want true")
	}
	aggregator.Merge(ResultAggregator{Count: 1, StatusCodes: map[string]int{"429": 1}, OutputKinds: map[string]int{"provider_error": 1}})

	summary := aggregator.Summary()
	if summary.Count != 3 {
		t.Fatalf("summary.Count = %d, want 3", summary.Count)
	}
	if summary.StatusCodes["200"] != 1 || summary.StatusCodes["503"] != 1 || summary.StatusCodes["429"] != 1 {
		t.Fatalf("status counts = %+v", summary.StatusCodes)
	}
	if summary.OutputKinds["text"] != 1 || summary.OutputKinds["provider_error"] != 2 {
		t.Fatalf("output kind counts = %+v", summary.OutputKinds)
	}
	if summary.FinishReasons["stop"] != 1 {
		t.Fatalf("finish reason counts = %+v", summary.FinishReasons)
	}
	if summary.ErrorKinds["api"] != 1 {
		t.Fatalf("error kind counts = %+v", summary.ErrorKinds)
	}
	if len(summary.RequestIDs) != 2 || summary.RequestIDs[0] != "req-1" || summary.RequestIDs[1] != "req-2" {
		t.Fatalf("request IDs = %+v", summary.RequestIDs)
	}
	if len(summary.Samples) != 2 {
		t.Fatalf("sample count = %d, want 2", len(summary.Samples))
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, want := range []string{`"count":3`, `"status_codes"`, `"output_kinds"`, `"error_kinds"`, `"samples"`} {
		if !json.Valid(encoded) || !strings.Contains(string(encoded), want) {
			t.Fatalf("encoded summary = %s, want fragment %s", encoded, want)
		}
	}
}
