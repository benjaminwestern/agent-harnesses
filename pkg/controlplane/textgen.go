package controlplane

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type CommitMessageInput struct {
	ModelSelection TextGenerationModelSelection
	Diff           string
	Instruction    string
	Metadata       map[string]any
}

type CommitMessageOutput struct {
	Message  string
	Metadata map[string]any
}

type PrContentInput struct {
	ModelSelection TextGenerationModelSelection
	Diff           string
	Title          string
	Instruction    string
	Metadata       map[string]any
}

type PrContentOutput struct {
	Title    string
	Body     string
	Metadata map[string]any
}

type BranchNameInput struct {
	ModelSelection TextGenerationModelSelection
	Summary        string
	Metadata       map[string]any
}

type BranchNameOutput struct {
	Name     string
	Metadata map[string]any
}

type ThreadTitleInput struct {
	ModelSelection TextGenerationModelSelection
	Prompt         string
	Metadata       map[string]any
}

type ThreadTitleOutput struct {
	Title    string
	Metadata map[string]any
}

type TextGenerationProvider interface {
	GenerateCommitMessage(context.Context, CommitMessageInput) (*CommitMessageOutput, error)
	GeneratePrContent(context.Context, PrContentInput) (*PrContentOutput, error)
	GenerateBranchName(context.Context, BranchNameInput) (*BranchNameOutput, error)
	GenerateThreadTitle(context.Context, ThreadTitleInput) (*ThreadTitleOutput, error)
}

type TextGenerationModelSelection struct {
	Provider  string
	Model     string
	Options   ModelOptions
	Fallbacks []string
}

type TextGenerationRouter struct {
	mu              sync.RWMutex
	defaultProvider string
	providers       map[string]TextGenerationProvider
}

func NewTextGenerationRouter(defaultProvider string, providers map[string]TextGenerationProvider) *TextGenerationRouter {
	router := &TextGenerationRouter{
		defaultProvider: normalizeProviderName(defaultProvider),
		providers:       make(map[string]TextGenerationProvider, len(providers)),
	}
	for name, provider := range providers {
		router.Register(name, provider)
	}
	return router
}

func (r *TextGenerationRouter) Register(providerName string, provider TextGenerationProvider) {
	if r == nil || provider == nil {
		return
	}
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[providerName] = provider
	if r.defaultProvider == "" {
		r.defaultProvider = providerName
	}
}

func (r *TextGenerationRouter) Route(providerName string) (TextGenerationProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("text generation router is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if provider := r.providers[normalizeProviderName(providerName)]; provider != nil {
		return provider, nil
	}
	if provider := r.providers[r.defaultProvider]; provider != nil {
		return provider, nil
	}
	return nil, fmt.Errorf("no text generation provider registered")
}

func (r *TextGenerationRouter) RouteSelection(selection TextGenerationModelSelection) (TextGenerationProvider, error) {
	resolved := r.ResolveSelection(selection)
	return r.Route(resolved.Provider)
}

func (r *TextGenerationRouter) ResolveSelection(selection TextGenerationModelSelection) TextGenerationModelSelection {
	resolved := selection
	resolved.Provider = r.resolveProviderName(selection)
	return resolved
}

func (r *TextGenerationRouter) Providers() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]string, 0, len(r.providers))
	for provider := range r.providers {
		providers = append(providers, provider)
	}
	return providers
}

func (r *TextGenerationRouter) GenerateCommitMessage(ctx context.Context, providerName string, input CommitMessageInput) (*CommitMessageOutput, error) {
	input.ModelSelection.Provider = coalesceProvider(providerName, input.ModelSelection.Provider)
	input.ModelSelection = r.ResolveSelection(input.ModelSelection)
	provider, err := r.Route(input.ModelSelection.Provider)
	if err != nil {
		return nil, err
	}
	return provider.GenerateCommitMessage(ctx, input)
}

func (r *TextGenerationRouter) GeneratePrContent(ctx context.Context, providerName string, input PrContentInput) (*PrContentOutput, error) {
	input.ModelSelection.Provider = coalesceProvider(providerName, input.ModelSelection.Provider)
	input.ModelSelection = r.ResolveSelection(input.ModelSelection)
	provider, err := r.Route(input.ModelSelection.Provider)
	if err != nil {
		return nil, err
	}
	return provider.GeneratePrContent(ctx, input)
}

func (r *TextGenerationRouter) GenerateBranchName(ctx context.Context, providerName string, input BranchNameInput) (*BranchNameOutput, error) {
	input.ModelSelection.Provider = coalesceProvider(providerName, input.ModelSelection.Provider)
	input.ModelSelection = r.ResolveSelection(input.ModelSelection)
	provider, err := r.Route(input.ModelSelection.Provider)
	if err != nil {
		return nil, err
	}
	return provider.GenerateBranchName(ctx, input)
}

func (r *TextGenerationRouter) GenerateThreadTitle(ctx context.Context, providerName string, input ThreadTitleInput) (*ThreadTitleOutput, error) {
	input.ModelSelection.Provider = coalesceProvider(providerName, input.ModelSelection.Provider)
	input.ModelSelection = r.ResolveSelection(input.ModelSelection)
	provider, err := r.Route(input.ModelSelection.Provider)
	if err != nil {
		return nil, err
	}
	return provider.GenerateThreadTitle(ctx, input)
}

func (r *TextGenerationRouter) GenerateCommitMessageForSelection(ctx context.Context, input CommitMessageInput) (*CommitMessageOutput, error) {
	return r.GenerateCommitMessage(ctx, "", input)
}

func (r *TextGenerationRouter) GeneratePrContentForSelection(ctx context.Context, input PrContentInput) (*PrContentOutput, error) {
	return r.GeneratePrContent(ctx, "", input)
}

func (r *TextGenerationRouter) GenerateBranchNameForSelection(ctx context.Context, input BranchNameInput) (*BranchNameOutput, error) {
	return r.GenerateBranchName(ctx, "", input)
}

func (r *TextGenerationRouter) GenerateThreadTitleForSelection(ctx context.Context, input ThreadTitleInput) (*ThreadTitleOutput, error) {
	return r.GenerateThreadTitle(ctx, "", input)
}

func (r *TextGenerationRouter) resolveProviderName(selection TextGenerationModelSelection) string {
	defaultProvider := ""
	if r != nil {
		defaultProvider = r.defaultProvider
	}
	for _, candidate := range providerCandidates(selection, defaultProvider) {
		normalized := normalizeProviderName(candidate)
		if normalized == "" {
			continue
		}
		if r == nil {
			return normalized
		}
		r.mu.RLock()
		provider := r.providers[normalized]
		r.mu.RUnlock()
		if provider != nil {
			return normalized
		}
	}
	return normalizeProviderName(selection.Provider)
}

func providerCandidates(selection TextGenerationModelSelection, defaultProvider string) []string {
	candidates := []string{
		selection.Provider,
		InferTextGenerationProvider(selection.Model),
	}
	candidates = append(candidates, selection.Fallbacks...)
	candidates = append(candidates, defaultProvider)
	return candidates
}

func InferTextGenerationProvider(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case model == "":
		return ""
	case strings.HasPrefix(model, "claude-"):
		return "claude"
	case strings.HasPrefix(model, "gemini-"), strings.HasPrefix(model, "auto-gemini-"):
		return "gemini"
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return "codex"
	case strings.Contains(model, "/"):
		return "opencode"
	default:
		return ""
	}
}

func normalizeProviderName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claudeagent", "claude-agent", "claude_code", "claudecode":
		return "claude"
	case "open-code":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func coalesceProvider(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
