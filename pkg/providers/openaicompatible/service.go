package openaicompatible

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/httpclient/openaicompat"
)

type Service struct {
	endpoint EndpointConfig
}

func NewService(endpoint EndpointConfig) *Service {
	return &Service{endpoint: endpoint}
}

func (s *Service) Endpoint() EndpointConfig {
	if s == nil {
		return EndpointConfig{}
	}
	return s.endpoint
}

func (s *Service) GenerateText(ctx context.Context, input api.GenerateTextInput) (*api.GenerateTextOutput, error) {
	started := time.Now()
	endpoint := s.endpointConfig()
	model := strings.TrimSpace(input.ModelSelection.Model)
	if model == "" {
		model = "ollama"
	}

	req := openaicompat.ChatCompletionRequest{
		Model:      model,
		Messages:   openAICompatibleMessages(input),
		Tools:      openAICompatibleTools(input.Tools),
		ToolChoice: input.ToolChoice,
		Stream:     false,
	}
	switch input.ResponseFormat {
	case "json", "json_object":
		req.ResponseFormat = &openaicompat.ResponseFormat{Type: "json_object"}
	case "text":
		req.ResponseFormat = &openaicompat.ResponseFormat{Type: "text"}
	}
	applyChatModelOptions(&req, input.ModelSelection.Options)

	resp, err := endpoint.client().CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible text generation failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned")
	}
	choice := resp.Choices[0]
	result := api.ProviderResultMetadata{
		Provider:      endpoint.providerName(),
		Model:         firstNonEmpty(resp.Model, model),
		LatencyMillis: time.Since(started).Milliseconds(),
		FinishReason:  choice.FinishReason,
		Usage:         providerUsage(resp.Usage, 0),
	}
	metadata := providerMetadata(result)
	return &api.GenerateTextOutput{
		Text:           openaicompat.MessageContentText(choice.Message.Content),
		Metadata:       metadata,
		Logprobs:       controlplaneLogprobs(choice.Logprobs),
		ProviderResult: result,
	}, nil
}

func (s *Service) GenerateEmbeddings(ctx context.Context, input api.EmbeddingInput) (*api.EmbeddingOutput, error) {
	started := time.Now()
	endpoint := s.endpointConfig()
	model := strings.TrimSpace(input.ModelSelection.Model)
	if model == "" {
		model = "nomic-embed-text"
	}
	dimensions := input.Dimensions
	if dimensions <= 0 {
		dimensions = input.ModelSelection.Dimensions
	}

	resp, err := endpoint.client().CreateEmbeddings(ctx, openaicompat.EmbeddingRequest{
		Model:      model,
		Input:      input.Texts,
		Dimensions: dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compatible embedding failed: %w", err)
	}

	vectors := make([][]float64, 0, len(resp.Data))
	for _, item := range resp.Data {
		vectors = append(vectors, item.Embedding)
	}
	result := api.ProviderResultMetadata{
		Provider:      endpoint.providerName(),
		Model:         firstNonEmpty(resp.Model, model),
		LatencyMillis: time.Since(started).Milliseconds(),
		Usage:         providerUsage(resp.Usage, len(vectors)),
	}
	metadata := providerMetadata(result)
	return &api.EmbeddingOutput{
		Vectors:        vectors,
		Metadata:       metadata,
		ProviderResult: result,
	}, nil
}

func (s *Service) ListModels(ctx context.Context) ([]contract.RuntimeModel, error) {
	endpoint := s.endpointConfig()
	return listOpenAICompatibleModelsWithError(ctx, endpoint.client(), endpoint.providerName())
}

func (s *Service) endpointConfig() EndpointConfig {
	if s == nil {
		return EndpointConfig{}
	}
	return s.endpoint
}

func (cfg EndpointConfig) providerName() string {
	if strings.TrimSpace(cfg.Provider) == "" {
		return runtimeName
	}
	return strings.TrimSpace(cfg.Provider)
}

func openAICompatibleMessages(input api.GenerateTextInput) []openaicompat.ChatMessage {
	if len(input.Messages) > 0 {
		messages := make([]openaicompat.ChatMessage, 0, len(input.Messages))
		for _, message := range input.Messages {
			messages = append(messages, openaicompat.ChatMessage{
				Role:       message.Role,
				Content:    message.Content,
				ToolCalls:  openAICompatibleToolCalls(message.ToolCalls),
				ToolCallID: message.ToolCallID,
				Name:       message.Name,
			})
		}
		return messages
	}
	messages := make([]openaicompat.ChatMessage, 0, 2)
	if input.SystemPrompt != "" {
		messages = append(messages, openaicompat.ChatMessage{Role: "system", Content: input.SystemPrompt})
	}
	if input.Prompt != "" {
		messages = append(messages, openaicompat.ChatMessage{Role: "user", Content: input.Prompt})
	}
	return messages
}

func openAICompatibleTools(tools []api.ToolDefinition) []openaicompat.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openaicompat.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openaicompat.ToolDefinition{
			Type: tool.Type,
			Function: openaicompat.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return out
}

func openAICompatibleToolCalls(calls []api.ToolCall) []openaicompat.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openaicompat.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, openaicompat.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: openaicompat.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	return out
}

func controlplaneLogprobs(logprobs *openaicompat.ChoiceLogprobs) []api.TokenLogprob {
	if logprobs == nil || len(logprobs.Content) == 0 {
		return nil
	}
	out := make([]api.TokenLogprob, 0, len(logprobs.Content))
	for _, item := range logprobs.Content {
		out = append(out, api.TokenLogprob{
			Token:   item.Token,
			Logprob: item.Logprob,
			Bytes:   item.Bytes,
		})
	}
	return out
}

func providerUsage(usage *openaicompat.Usage, vectorCount int) api.ProviderUsage {
	out := api.ProviderUsage{VectorCount: vectorCount}
	if usage == nil {
		return out
	}
	out.PromptTokens = usage.PromptTokens
	out.CompletionTokens = usage.CompletionTokens
	out.TotalTokens = usage.TotalTokens
	return out
}

func providerMetadata(result api.ProviderResultMetadata) map[string]any {
	return map[string]any{
		"provider":        result.Provider,
		"model":           result.Model,
		"usage":           result.Usage,
		"provider_result": result,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
