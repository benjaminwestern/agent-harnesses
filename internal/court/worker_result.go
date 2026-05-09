// Package court provides Court runtime functionality.
package court

import (
	"encoding/json"
	"fmt"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func WorkerResultSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "number"},
			"summary":        map[string]any{"type": "string"},
			"findings":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"risks":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"next_actions":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"confidence":     map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"verdict":        map[string]any{"type": "string"},
		},
		"required":             []string{"schema_version", "summary", "findings", "risks", "next_actions", "confidence", "verdict"},
		"additionalProperties": false,
	}
}

func readResultFromOutput(output string) (string, string, error) {
	return api.ExtractStructuredJSON(output, parseWorkerResult)
}

type judgeEvidenceItem struct {
	WorkerID  string          `json:"worker_id"`
	RoleID    string          `json:"role_id"`
	RoleKind  RoleKind        `json:"role_kind"`
	RoleTitle string          `json:"role_title"`
	Result    json.RawMessage `json:"result"`
}

func judgeEvidenceJSON(workers []Worker) string {
	items := make([]judgeEvidenceItem, 0, len(workers))
	for _, worker := range workers {
		if worker.RoleKind == RoleJudge || worker.Status != WorkerCompleted || strings.TrimSpace(worker.ResultJSON) == "" {
			continue
		}
		if _, normalised, err := parseWorkerResult(worker.ResultJSON); err == nil {
			worker.ResultJSON = normalised
		} else {
			continue
		}
		items = append(items, judgeEvidenceItem{
			WorkerID:  worker.ID,
			RoleID:    worker.RoleID,
			RoleKind:  worker.RoleKind,
			RoleTitle: worker.RoleTitle,
			Result:    json.RawMessage(worker.ResultJSON),
		})
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func parseWorkerResult(raw string) (string, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return "", "", fmt.Errorf("invalid json: %w", err)
	}
	for _, key := range WorkerResultRequiredFields() {
		if _, ok := fields[key]; !ok {
			return "", "", fmt.Errorf("missing field: %s", key)
		}
	}
	if len(fields) != len(WorkerResultRequiredFields()) {
		return "", "", fmt.Errorf("extra fields")
	}

	var parsed WorkerResult
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", "", fmt.Errorf("parse error: %w", err)
	}
	if parsed.SchemaVersion != 1 || !meaningfulWorkerResultText(parsed.Summary) || !meaningfulWorkerResultText(parsed.Verdict) {
		return "", "", fmt.Errorf("invalid schema or summary/verdict")
	}
	if !validStringList(parsed.Findings) || !validStringList(parsed.Risks) || !validStringList(parsed.NextActions) {
		return "", "", fmt.Errorf("invalid string list")
	}
	if !ValidWorkerResultConfidence(parsed.Confidence) {
		return "", "", fmt.Errorf("invalid confidence")
	}
	normalised, _ := json.Marshal(parsed)
	var b strings.Builder
	if parsed.Summary != "" {
		b.WriteString(parsed.Summary)
		b.WriteString("\n\n")
	}
	if len(parsed.Findings) > 0 {
		b.WriteString("Findings:\n")
		for _, item := range parsed.Findings {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(parsed.Risks) > 0 {
		b.WriteString("Risks:\n")
		for _, item := range parsed.Risks {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(parsed.NextActions) > 0 {
		b.WriteString("Next actions:\n")
		for _, item := range parsed.NextActions {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if parsed.Verdict != "" {
		b.WriteString("Verdict:\n")
		b.WriteString(parsed.Verdict)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), string(normalised), nil
}

func validStringList(values []string) bool {
	if values == nil {
		return false
	}
	for _, value := range values {
		if !meaningfulWorkerResultText(value) {
			return false
		}
	}
	return true
}

func meaningfulWorkerResultText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "..." {
		return false
	}
	return !strings.HasPrefix(value, "<") || !strings.HasSuffix(value, ">")
}
