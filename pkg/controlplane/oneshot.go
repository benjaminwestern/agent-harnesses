package controlplane

import (
	"context"
	"slices"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

// OneShot exposes request/response model calls without requiring callers to
// adopt session, workspace, or project ownership from the control plane.
type OneShot struct {
	textGen            *TextGenerationRouter
	embeddings         *EmbeddingRouter
	textProviders      map[string]TextGenerationProvider
	embeddingProviders map[string]EmbeddingProvider
}

func NewOneShot(textDefaultProvider string, textProviders map[string]TextGenerationProvider, embeddingDefaultProvider string, embeddingProviders map[string]EmbeddingProvider) *OneShot {
	return &OneShot{
		textGen:            NewTextGenerationRouter(textDefaultProvider, textProviders),
		embeddings:         NewEmbeddingRouter(embeddingDefaultProvider, embeddingProviders),
		textProviders:      copyTextProviders(textProviders),
		embeddingProviders: copyEmbeddingProviders(embeddingProviders),
	}
}

func (o *OneShot) TextGen() *TextGenerationRouter {
	if o == nil {
		return nil
	}
	return o.textGen
}

func (o *OneShot) Embeddings() *EmbeddingRouter {
	if o == nil {
		return nil
	}
	return o.embeddings
}

func (o *OneShot) GenerateText(ctx context.Context, input GenerateTextInput) (*GenerateTextOutput, error) {
	if o == nil || o.textGen == nil {
		return nil, contextError("one-shot text generation router is nil")
	}
	return o.textGen.GenerateTextForSelection(ctx, input)
}

func (o *OneShot) GenerateTextWithOptions(ctx context.Context, opts GenerateOptions, messages []Message) (*GenerateTextOutput, error) {
	if o == nil || o.textGen == nil {
		return nil, contextError("one-shot text generation router is nil")
	}
	return o.textGen.GenerateTextWithOptions(ctx, opts, messages)
}

func (o *OneShot) GenerateEmbeddings(ctx context.Context, input EmbeddingInput) (*EmbeddingOutput, error) {
	if o == nil || o.embeddings == nil {
		return nil, contextError("one-shot embedding router is nil")
	}
	return o.embeddings.GenerateEmbeddingsForSelection(ctx, input)
}

func (o *OneShot) Capabilities(query CapabilityQuery) TaskCapability {
	if o == nil {
		return ProviderCapabilities(query.Provider, query.Model)
	}
	return mergeTaskCapabilities(
		o.textGen.Capabilities(query),
		o.embeddings.Capabilities(query),
	)
}

func (o *OneShot) Describe() contract.SystemDescriptor {
	return contract.SystemDescriptor{
		SchemaVersion:       contract.ControlPlaneSchemaVersion,
		WireProtocolVersion: contract.WireProtocolVersion,
		Runtimes:            o.RuntimeDescriptors(),
	}
}

func (o *OneShot) ModelRegistry() contract.ModelRegistry {
	return BuildModelRegistry(o.RuntimeDescriptors())
}

func (o *OneShot) RuntimeDescriptors() []contract.RuntimeDescriptor {
	if o == nil {
		return nil
	}
	descriptors := make(map[string]contract.RuntimeDescriptor)
	for name, provider := range o.textProviders {
		descriptor := describeOneShotProvider(name, provider)
		descriptor.Capabilities.TextGeneration = true
		descriptors[descriptor.Runtime] = mergeOneShotDescriptor(descriptors[descriptor.Runtime], descriptor)
	}
	for name, provider := range o.embeddingProviders {
		descriptor := describeOneShotProvider(name, provider)
		descriptor.Capabilities.Embeddings = true
		descriptors[descriptor.Runtime] = mergeOneShotDescriptor(descriptors[descriptor.Runtime], descriptor)
	}
	out := make([]contract.RuntimeDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		out = append(out, descriptor)
	}
	slices.SortFunc(out, func(left, right contract.RuntimeDescriptor) int {
		if left.Runtime < right.Runtime {
			return -1
		}
		if left.Runtime > right.Runtime {
			return 1
		}
		return 0
	})
	return out
}

type runtimeDescriber interface {
	Describe() contract.RuntimeDescriptor
}

func describeOneShotProvider(name string, provider any) contract.RuntimeDescriptor {
	if describer, ok := provider.(runtimeDescriber); ok {
		descriptor := describer.Describe()
		if descriptor.Runtime == "" {
			descriptor.Runtime = name
		}
		return descriptor
	}
	return contract.NewRuntimeDescriptor(name, contract.OwnershipObserved, contract.TransportRPC, contract.RuntimeCapabilities{})
}

func mergeOneShotDescriptor(existing contract.RuntimeDescriptor, next contract.RuntimeDescriptor) contract.RuntimeDescriptor {
	if existing.Runtime == "" {
		return next
	}
	existing.Capabilities.TextGeneration = existing.Capabilities.TextGeneration || next.Capabilities.TextGeneration
	existing.Capabilities.Embeddings = existing.Capabilities.Embeddings || next.Capabilities.Embeddings
	if existing.Probe == nil {
		existing.Probe = next.Probe
	} else if next.Probe != nil {
		existing.Probe.Models = mergeRuntimeModels(existing.Runtime, existing.Probe.Models, next.Probe.Models)
		existing.Probe.Installed = existing.Probe.Installed || next.Probe.Installed
		if existing.Probe.ModelSource == "" {
			existing.Probe.ModelSource = next.Probe.ModelSource
		}
		if existing.Probe.Message == "" {
			existing.Probe.Message = next.Probe.Message
		}
	}
	return existing
}

func copyTextProviders(providers map[string]TextGenerationProvider) map[string]TextGenerationProvider {
	out := make(map[string]TextGenerationProvider, len(providers))
	for name, provider := range providers {
		out[name] = provider
	}
	return out
}

func copyEmbeddingProviders(providers map[string]EmbeddingProvider) map[string]EmbeddingProvider {
	out := make(map[string]EmbeddingProvider, len(providers))
	for name, provider := range providers {
		out[name] = provider
	}
	return out
}
