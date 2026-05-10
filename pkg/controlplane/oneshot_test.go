package controlplane

import (
	"context"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type oneShotTextProvider struct{}

func (oneShotTextProvider) Describe() contract.RuntimeDescriptor {
	return contract.RuntimeDescriptor{
		Runtime: "fixture-text",
		Probe: &contract.RuntimeProbe{
			Installed: true,
			Models: []contract.RuntimeModel{{
				ID:       "fixture-chat",
				Provider: "fixture",
			}},
		},
	}
}

func (oneShotTextProvider) GenerateCommitMessage(context.Context, CommitMessageInput) (*CommitMessageOutput, error) {
	return &CommitMessageOutput{}, nil
}

func (oneShotTextProvider) GeneratePrContent(context.Context, PrContentInput) (*PrContentOutput, error) {
	return &PrContentOutput{}, nil
}

func (oneShotTextProvider) GenerateBranchName(context.Context, BranchNameInput) (*BranchNameOutput, error) {
	return &BranchNameOutput{}, nil
}

func (oneShotTextProvider) GenerateThreadTitle(context.Context, ThreadTitleInput) (*ThreadTitleOutput, error) {
	return &ThreadTitleOutput{}, nil
}

func (oneShotTextProvider) GenerateText(context.Context, GenerateTextInput) (*GenerateTextOutput, error) {
	return &GenerateTextOutput{Text: "ok"}, nil
}

type oneShotEmbeddingProvider struct{}

func (oneShotEmbeddingProvider) Describe() contract.RuntimeDescriptor {
	return contract.RuntimeDescriptor{
		Runtime: "fixture-embedding",
		Probe: &contract.RuntimeProbe{
			Installed: true,
			Models: []contract.RuntimeModel{{
				ID:       "fixture-embedding",
				Provider: "fixture",
			}},
		},
	}
}

func (oneShotEmbeddingProvider) GenerateEmbeddings(context.Context, EmbeddingInput) (*EmbeddingOutput, error) {
	return &EmbeddingOutput{Vectors: [][]float64{{1, 2, 3}}}, nil
}

func TestOneShotExposesModelRegistry(t *testing.T) {
	oneShot := NewOneShot(
		"fixture-text",
		map[string]TextGenerationProvider{"fixture-text": oneShotTextProvider{}},
		"fixture-embedding",
		map[string]EmbeddingProvider{"fixture-embedding": oneShotEmbeddingProvider{}},
	)
	registry := oneShot.ModelRegistry()
	if len(registry.Backends) != 2 {
		t.Fatalf("backend count = %d, want 2", len(registry.Backends))
	}
	byBackend := map[string]contract.RuntimeBackendRegistry{}
	for _, backend := range registry.Backends {
		byBackend[backend.Backend] = backend
	}
	if !byBackend["fixture-text"].SupportsTextGeneration {
		t.Fatalf("text backend capabilities = %#v, want text generation", byBackend["fixture-text"])
	}
	if !byBackend["fixture-embedding"].SupportsEmbeddings {
		t.Fatalf("embedding backend capabilities = %#v, want embeddings", byBackend["fixture-embedding"])
	}
}
