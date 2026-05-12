package controlplane

import (
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

// TaskCapability describes provider capabilities at the control-plane
// provider-client boundary.
type TaskCapability struct {
	TextGeneration bool `json:"text_generation"`
	Embeddings     bool `json:"embeddings"`
	Vision         bool `json:"vision"`
	Streaming      bool `json:"streaming"`
}

// CapabilityQuery selects provider capabilities. Model is optional because
// some providers expose capabilities at provider level while others vary by
// model.
type CapabilityQuery struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ProviderCapabilities returns convention-first capabilities for known
// downstream providers without requiring callers to construct a router.
func ProviderCapabilities(provider string, model string) TaskCapability {
	switch normalizeCapabilityProvider(firstNonEmptyString(provider, InferRuntimeProvider(model))) {
	case "openai-compatible", "openai", "ollama":
		return TaskCapability{TextGeneration: true, Embeddings: true, Vision: true}
	case "codex", "claude", "gemini", "opencode", "pi":
		return TaskCapability{TextGeneration: true}
	default:
		return TaskCapability{}
	}
}

func (r *TextGenerationRouter) Capabilities(query CapabilityQuery) TaskCapability {
	providerName := r.resolveProviderName(TextGenerationModelSelection{
		Provider: query.Provider,
		Model:    query.Model,
	})
	caps := ProviderCapabilities(providerName, query.Model)
	provider := r.textProvider(providerName)
	if provider == nil {
		return caps
	}
	caps = mergeTaskCapabilities(caps, providerRuntimeCapabilities(provider, query))
	caps.TextGeneration = true
	return caps
}

func (r *EmbeddingRouter) Capabilities(query CapabilityQuery) TaskCapability {
	providerName := r.resolveProviderName(EmbeddingModelSelection{
		Provider: query.Provider,
		Model:    query.Model,
	})
	caps := ProviderCapabilities(providerName, query.Model)
	provider := r.embeddingProvider(providerName)
	if provider == nil {
		return caps
	}
	caps = mergeTaskCapabilities(caps, providerRuntimeCapabilities(provider, query))
	caps.Embeddings = true
	return caps
}

func normalizeCapabilityProvider(provider string) string {
	switch NormalizeRuntimeBackend(provider) {
	case "openaicompatible", "openai_compatible":
		return "openai-compatible"
	default:
		return NormalizeRuntimeBackend(provider)
	}
}

func mergeTaskCapabilities(left TaskCapability, right TaskCapability) TaskCapability {
	return TaskCapability{
		TextGeneration: left.TextGeneration || right.TextGeneration,
		Embeddings:     left.Embeddings || right.Embeddings,
		Vision:         left.Vision || right.Vision,
		Streaming:      left.Streaming || right.Streaming,
	}
}

func (r *TextGenerationRouter) textProvider(providerName string) TextGenerationProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[NormalizeRuntimeBackend(providerName)]
}

func (r *EmbeddingRouter) embeddingProvider(providerName string) EmbeddingProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[NormalizeRuntimeBackend(providerName)]
}

func providerRuntimeCapabilities(provider any, query CapabilityQuery) TaskCapability {
	describer, ok := provider.(runtimeDescriber)
	if !ok {
		return TaskCapability{}
	}
	return descriptorCapabilities(describer.Describe(), query)
}

func descriptorCapabilities(descriptor contract.RuntimeDescriptor, query CapabilityQuery) TaskCapability {
	caps := TaskCapability{
		TextGeneration: descriptor.Capabilities.TextGeneration,
		Embeddings:     descriptor.Capabilities.Embeddings,
	}
	if descriptor.Probe == nil {
		return caps
	}
	model := strings.TrimSpace(query.Model)
	for _, item := range descriptor.Probe.Models {
		if model != "" && item.ID != model {
			continue
		}
		caps = mergeTaskCapabilities(caps, runtimeModelTaskCapabilities(item))
		if model != "" {
			return caps
		}
	}
	return caps
}

func runtimeModelTaskCapabilities(model contract.RuntimeModel) TaskCapability {
	var caps TaskCapability
	for _, task := range model.Capabilities.Tasks {
		switch task {
		case contract.RuntimeModelTaskTextGeneration:
			caps.TextGeneration = true
		case contract.RuntimeModelTaskEmbeddings:
			caps.Embeddings = true
		}
	}
	for _, modality := range model.Capabilities.InputModalities {
		if modality == contract.RuntimeModelModalityImage {
			caps.Vision = true
			break
		}
	}
	return caps
}
