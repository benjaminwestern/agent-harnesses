package openaicompatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func TestServiceGenerateTextPassesControlsAndTools(t *testing.T) {
	var got struct {
		Model       string               `json:"model"`
		MaxTokens   int                  `json:"max_tokens"`
		Temperature *float64             `json:"temperature"`
		TopP        *float64             `json:"top_p"`
		Tools       []api.ToolDefinition `json:"tools"`
		Messages    []api.Message        `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-fixture",
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "done"},
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	defer server.Close()

	temp := 0.2
	topP := 0.8
	out, err := NewService(EndpointConfig{Provider: "openai", BaseURL: server.URL}).GenerateText(context.Background(), api.GenerateTextInput{
		ModelSelection: api.TextGenerationModelSelection{
			Provider: "openai",
			Model:    "gpt-fixture",
			Options: api.ModelOptions{
				MaxOutputTokens: 64,
				Temperature:     &temp,
				TopP:            &topP,
			},
		},
		Messages: []api.Message{{Role: "user", Content: "hello"}},
		Tools: []api.ToolDefinition{{
			Type: "function",
			Function: api.FunctionDefinition{
				Name:       "lookup",
				Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if got.Model != "gpt-fixture" || got.MaxTokens != 64 || got.Temperature == nil || *got.Temperature != temp || got.TopP == nil || *got.TopP != topP {
		t.Fatalf("controls not passed: %+v", got)
	}
	if len(got.Tools) != 1 || got.Tools[0].Function.Name != "lookup" {
		t.Fatalf("tools not passed: %+v", got.Tools)
	}
	if out.Text != "done" || out.ProviderResult.Provider != "openai" || out.ProviderResult.Usage.PromptTokens != 3 || out.ProviderResult.Usage.CompletionTokens != 2 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestServiceGenerateEmbeddingsPassesDimensions(t *testing.T) {
	var got struct {
		Model      string   `json:"model"`
		Input      []string `json:"input"`
		Dimensions int      `json:"dimensions"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "embed-fixture",
			"data":  []map[string]any{{"embedding": []float64{1, 2, 3}}},
		})
	}))
	defer server.Close()

	out, err := NewService(EndpointConfig{Provider: "openai-compatible", BaseURL: server.URL}).GenerateEmbeddings(context.Background(), api.EmbeddingInput{
		ModelSelection: api.EmbeddingModelSelection{
			Provider: "openai-compatible",
			Model:    "embed-fixture",
		},
		Texts:      []string{"alpha"},
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("GenerateEmbeddings: %v", err)
	}
	if got.Model != "embed-fixture" || got.Dimensions != 3 || len(got.Input) != 1 || got.Input[0] != "alpha" {
		t.Fatalf("unexpected request: %+v", got)
	}
	if len(out.Vectors) != 1 || len(out.Vectors[0]) != 3 || out.ProviderResult.Usage.VectorCount != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
}
