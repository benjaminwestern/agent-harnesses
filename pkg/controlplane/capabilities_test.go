package controlplane

import (
	"context"
	"testing"
)

type capabilityTextProvider struct{}

func (capabilityTextProvider) GenerateCommitMessage(context.Context, CommitMessageInput) (*CommitMessageOutput, error) {
	return &CommitMessageOutput{}, nil
}

func (capabilityTextProvider) GeneratePrContent(context.Context, PrContentInput) (*PrContentOutput, error) {
	return &PrContentOutput{}, nil
}

func (capabilityTextProvider) GenerateBranchName(context.Context, BranchNameInput) (*BranchNameOutput, error) {
	return &BranchNameOutput{}, nil
}

func (capabilityTextProvider) GenerateThreadTitle(context.Context, ThreadTitleInput) (*ThreadTitleOutput, error) {
	return &ThreadTitleOutput{}, nil
}

func (capabilityTextProvider) GenerateText(context.Context, GenerateTextInput) (*GenerateTextOutput, error) {
	return &GenerateTextOutput{}, nil
}

type capabilityEmbeddingProvider struct{}

func (capabilityEmbeddingProvider) GenerateEmbeddings(context.Context, EmbeddingInput) (*EmbeddingOutput, error) {
	return &EmbeddingOutput{}, nil
}

func TestProviderCapabilitiesKnownProviders(t *testing.T) {
	openAICompat := ProviderCapabilities("openai-compatible", "gpt-fixture")
	if !openAICompat.TextGeneration || !openAICompat.Embeddings || !openAICompat.Vision {
		t.Fatalf("openai-compatible capabilities = %+v, want text embeddings vision", openAICompat)
	}

	ollama := ProviderCapabilities("ollama", "llava")
	if !ollama.TextGeneration || !ollama.Embeddings || !ollama.Vision {
		t.Fatalf("ollama capabilities = %+v, want text embeddings vision", ollama)
	}

	codex := ProviderCapabilities("codex", "gpt-5.4")
	if !codex.TextGeneration || codex.Embeddings || codex.Vision {
		t.Fatalf("codex capabilities = %+v, want text-only", codex)
	}
}

func TestRoutersExposeCapabilitiesForRegisteredProviders(t *testing.T) {
	textRouter := NewTextGenerationRouter("codex", map[string]TextGenerationProvider{
		"codex": capabilityTextProvider{},
	})
	textCaps := textRouter.Capabilities(CapabilityQuery{Provider: "codex"})
	if !textCaps.TextGeneration || textCaps.Embeddings || textCaps.Vision {
		t.Fatalf("text router capabilities = %+v, want text-only", textCaps)
	}

	embeddingRouter := NewEmbeddingRouter("fixture-embeddings", map[string]EmbeddingProvider{
		"fixture-embeddings": capabilityEmbeddingProvider{},
	})
	embeddingCaps := embeddingRouter.Capabilities(CapabilityQuery{Provider: "fixture-embeddings"})
	if embeddingCaps.TextGeneration || !embeddingCaps.Embeddings || embeddingCaps.Vision {
		t.Fatalf("embedding router capabilities = %+v, want embeddings-only", embeddingCaps)
	}
}

func TestOneShotMergesCapabilitiesAcrossRouters(t *testing.T) {
	oneShot := NewOneShot(
		"openai-compatible",
		map[string]TextGenerationProvider{"openai-compatible": capabilityTextProvider{}},
		"openai-compatible",
		map[string]EmbeddingProvider{"openai-compatible": capabilityEmbeddingProvider{}},
	)
	caps := oneShot.Capabilities(CapabilityQuery{Provider: "openai-compatible"})
	if !caps.TextGeneration || !caps.Embeddings || !caps.Vision {
		t.Fatalf("one-shot capabilities = %+v, want text embeddings vision", caps)
	}
}
