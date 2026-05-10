package builtin

import (
	"github.com/benjaminwestern/agentic-control/internal/config"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/providers/claude"
	"github.com/benjaminwestern/agentic-control/pkg/providers/codex"
	"github.com/benjaminwestern/agentic-control/pkg/providers/gemini"
	"github.com/benjaminwestern/agentic-control/pkg/providers/openaicompatible"
	"github.com/benjaminwestern/agentic-control/pkg/providers/opencode"
	"github.com/benjaminwestern/agentic-control/pkg/providers/pi"
)

// NewOneShot exposes all built-in downstream providers through request/response
// routers without constructing the internal session/workspace control-plane
// service.
func NewOneShot() *api.OneShot {
	cfg := config.Load()
	emit := func(contract.RuntimeEvent) {}
	textProviders := map[string]api.TextGenerationProvider{
		"codex":             codex.NewProvider(emit, cfg.Runtimes["codex"]),
		"claude":            claude.NewProvider(emit, cfg.Runtimes["claude"]),
		"gemini":            gemini.NewProvider(emit, cfg.Runtimes["gemini"]),
		"opencode":          opencode.NewProvider(emit, cfg.Runtimes["opencode"]),
		"pi":                pi.NewProvider(emit, cfg.Runtimes["pi"]),
		"openai-compatible": openaicompatible.NewProvider(emit, cfg.Runtimes["openai-compatible"]),
	}
	embeddingProviders := map[string]api.EmbeddingProvider{
		"openai-compatible": openaicompatible.NewProvider(emit, cfg.Runtimes["openai-compatible"]),
	}
	return api.NewOneShot("codex", textProviders, "openai-compatible", embeddingProviders)
}
