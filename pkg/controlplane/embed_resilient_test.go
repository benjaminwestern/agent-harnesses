package controlplane

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
)

type resilientEmbeddingProvider struct {
	mu    sync.Mutex
	calls [][]string
	embed func([]string) (*EmbeddingOutput, error)
}

func (p *resilientEmbeddingProvider) GenerateEmbeddings(_ context.Context, input EmbeddingInput) (*EmbeddingOutput, error) {
	p.mu.Lock()
	p.calls = append(p.calls, append([]string(nil), input.Texts...))
	p.mu.Unlock()
	return p.embed(input.Texts)
}

func (p *resilientEmbeddingProvider) callSnapshot() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]string, len(p.calls))
	for i, call := range p.calls {
		out[i] = append([]string(nil), call...)
	}
	return out
}

func TestResilientEmbedBatchDeduplicatesWithCacheAndPreservesOrder(t *testing.T) {
	provider := &resilientEmbeddingProvider{
		embed: func(texts []string) (*EmbeddingOutput, error) {
			vectors := make([][]float64, 0, len(texts))
			for _, text := range texts {
				vectors = append(vectors, []float64{float64(len(text))})
			}
			return &EmbeddingOutput{
				Vectors:        vectors,
				ProviderResult: ProviderResultMetadata{StatusCode: 200, OutputKind: "embeddings"},
			}, nil
		},
	}

	out, err := ResilientEmbedBatch(context.Background(), provider, ResilientEmbedConfig{EnableCache: true}, EmbeddingInput{
		ModelSelection: EmbeddingModelSelection{Provider: "fixture", Model: "embedder"},
		Texts:          []string{"alpha", "beta", "alpha"},
	})
	if err != nil {
		t.Fatalf("ResilientEmbedBatch: %v", err)
	}
	if got := provider.callSnapshot(); len(got) != 1 || !slices.Equal(got[0], []string{"alpha", "beta"}) {
		t.Fatalf("provider calls = %+v, want one call with unique texts", got)
	}
	if len(out.Vectors) != 3 || out.Vectors[0][0] != 5 || out.Vectors[1][0] != 4 || out.Vectors[2][0] != 5 {
		t.Fatalf("vectors = %+v", out.Vectors)
	}
	if !slices.Equal(out.Reused, []bool{false, false, true}) {
		t.Fatalf("reused = %+v, want duplicate marked reused", out.Reused)
	}
	if out.ProviderResult.Count != 1 || out.ProviderResult.OutputKinds["embeddings"] != 1 {
		t.Fatalf("provider summary = %+v", out.ProviderResult)
	}
}

func TestResilientEmbedBatchSplitsRetryableBatchFailures(t *testing.T) {
	provider := &resilientEmbeddingProvider{
		embed: func(texts []string) (*EmbeddingOutput, error) {
			if len(texts) > 1 {
				return nil, NewProviderResultError("busy", ProviderResultMetadata{
					StatusCode: 503,
					OutputKind: "provider_error",
					Error:      &ProviderError{Kind: "api", Retryable: true},
				}, nil)
			}
			return &EmbeddingOutput{
				Vectors:        [][]float64{{float64(texts[0][0])}},
				ProviderResult: ProviderResultMetadata{StatusCode: 200, OutputKind: "embeddings"},
			}, nil
		},
	}

	out, err := ResilientEmbedBatch(context.Background(), provider, ResilientEmbedConfig{MaxRetries: 4}, EmbeddingInput{
		ModelSelection: EmbeddingModelSelection{Provider: "fixture", Model: "embedder"},
		Texts:          []string{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("ResilientEmbedBatch: %v", err)
	}
	if len(out.Vectors) != 3 || out.Vectors[0][0] != 'a' || out.Vectors[1][0] != 'b' || out.Vectors[2][0] != 'c' {
		t.Fatalf("vectors = %+v", out.Vectors)
	}
	if out.ProviderResult.StatusCodes["503"] != 2 || out.ProviderResult.StatusCodes["200"] != 3 {
		t.Fatalf("status summary = %+v", out.ProviderResult.StatusCodes)
	}
}

func TestResilientEmbedBatchDoesNotSplitTerminalFailures(t *testing.T) {
	provider := &resilientEmbeddingProvider{
		embed: func(texts []string) (*EmbeddingOutput, error) {
			return nil, NewProviderResultError("bad request", ProviderResultMetadata{
				StatusCode: 400,
				OutputKind: "provider_error",
				Error:      &ProviderError{Kind: "invalid_request", Retryable: false},
			}, nil)
		},
	}

	_, err := ResilientEmbedBatch(context.Background(), provider, ResilientEmbedConfig{MaxRetries: 4}, EmbeddingInput{
		ModelSelection: EmbeddingModelSelection{Provider: "fixture", Model: "embedder"},
		Texts:          []string{"a", "b", "c"},
	})
	if err == nil {
		t.Fatal("ResilientEmbedBatch succeeded, want terminal provider error")
	}
	var resultErr *ProviderResultError
	if !errors.As(err, &resultErr) || resultErr.Result.StatusCode != 400 {
		t.Fatalf("error = %T %+v, want ProviderResultError with 400", err, err)
	}
	if got := provider.callSnapshot(); len(got) != 1 {
		t.Fatalf("provider calls = %+v, want no retry splitting for terminal error", got)
	}
}

func TestResilientEmbedBatchSplitsOversizedSingletons(t *testing.T) {
	provider := &resilientEmbeddingProvider{
		embed: func(texts []string) (*EmbeddingOutput, error) {
			vectors := make([][]float64, 0, len(texts))
			for _, text := range texts {
				if len(text) > 4 {
					return nil, NewProviderResultError("too large", ProviderResultMetadata{
						StatusCode: 429,
						OutputKind: "provider_error",
						Error:      &ProviderError{Kind: "api", Retryable: true},
					}, nil)
				}
				vectors = append(vectors, []float64{float64(text[0])})
			}
			return &EmbeddingOutput{
				Vectors:        vectors,
				ProviderResult: ProviderResultMetadata{StatusCode: 200, OutputKind: "embeddings"},
			}, nil
		},
	}

	out, err := ResilientEmbedBatch(context.Background(), provider, ResilientEmbedConfig{MaxRetries: 3, SplitOversized: true}, EmbeddingInput{
		ModelSelection: EmbeddingModelSelection{Provider: "fixture", Model: "embedder"},
		Texts:          []string{"abcdef"},
	})
	if err != nil {
		t.Fatalf("ResilientEmbedBatch: %v", err)
	}
	if len(out.Vectors) != 1 || out.Vectors[0][0] != 98.5 {
		t.Fatalf("vector = %+v, want weighted average of split parts", out.Vectors)
	}
	if out.ProviderResult.StatusCodes["429"] != 1 || out.ProviderResult.StatusCodes["200"] != 1 {
		t.Fatalf("status summary = %+v", out.ProviderResult.StatusCodes)
	}
}

func TestResilientEmbedBatchReportsProgress(t *testing.T) {
	provider := &resilientEmbeddingProvider{
		embed: func(texts []string) (*EmbeddingOutput, error) {
			vectors := make([][]float64, 0, len(texts))
			for range texts {
				vectors = append(vectors, []float64{1})
			}
			return &EmbeddingOutput{
				Vectors:        vectors,
				ProviderResult: ProviderResultMetadata{StatusCode: 200, OutputKind: "embeddings"},
			}, nil
		},
	}
	var phases []string
	_, err := ResilientEmbedBatch(context.Background(), provider, ResilientEmbedConfig{
		EnableCache: true,
		Progress: func(progress ResilientEmbedProgress) error {
			phases = append(phases, progress.Phase)
			return nil
		},
	}, EmbeddingInput{
		ModelSelection: EmbeddingModelSelection{Provider: "fixture", Model: "embedder"},
		Texts:          []string{"same", "same"},
	})
	if err != nil {
		t.Fatalf("ResilientEmbedBatch: %v", err)
	}
	if !slices.Contains(phases, "cache_hit") || !slices.Contains(phases, "batch_start") || !slices.Contains(phases, "batch_success") {
		t.Fatalf("progress phases = %+v, want cache_hit, batch_start, batch_success", phases)
	}
}
