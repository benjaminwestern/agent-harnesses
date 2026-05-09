package gemini

import (
	"context"
	"strings"
	"testing"

	"github.com/benjaminwestern/agentic-control/internal/config"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func TestSendInputRejectsContentParts(t *testing.T) {
	provider := NewProvider(func(contract.RuntimeEvent) {}, config.RuntimeConfig{})
	provider.sessions["session-1"] = &session{appSessionID: "session-1", provider: provider}

	_, err := provider.SendInput(context.Background(), api.SendInputRequest{
		SessionID: "session-1",
		Parts: []contract.ContentPart{{
			Type: contract.ContentPartTypeImage,
			URL:  "https://example.com/image.png",
		}},
	})
	if err == nil {
		t.Fatal("SendInput succeeded, want unsupported parts error")
	}
	if !strings.Contains(err.Error(), "does not support multimodal content parts") {
		t.Fatalf("error = %q", err.Error())
	}
}
