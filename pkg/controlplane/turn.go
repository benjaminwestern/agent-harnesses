package controlplane

import (
	"encoding/json"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type TurnAccumulator struct {
	EventLines  []string
	JoinedText  strings.Builder
	LatestDelta string
	FinalText   string
}

func (t *TurnAccumulator) Add(event contract.RuntimeEvent) {
	if encoded, err := json.Marshal(event); err == nil {
		t.EventLines = append(t.EventLines, string(encoded))
	}
	if delta := contract.EventDeltaText(event); delta != "" {
		t.LatestDelta = delta
		t.JoinedText.WriteString(delta)
	}
	if finalText := contract.EventFinalText(event); finalText != "" {
		t.FinalText = finalText
	}
}

func (t *TurnAccumulator) JoinedDelta() string {
	return strings.TrimSpace(t.JoinedText.String())
}

func (t *TurnAccumulator) EventsJSONL() string {
	return strings.Join(t.EventLines, "\n")
}

func (t *TurnAccumulator) HasEvents() bool {
	return len(t.EventLines) > 0
}
