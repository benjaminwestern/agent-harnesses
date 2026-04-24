// Package court provides Court runtime functionality.
package court

import (
	"encoding/json"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func readResultFromOutput(output string) (string, string) {
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
		if _, normalised, ok := parseWorkerResult(worker.ResultJSON); ok {
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

func parseWorkerResult(raw string) (string, string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return "", "", false
	}
	for _, key := range WorkerResultRequiredFields() {
		if _, ok := fields[key]; !ok {
			return "", "", false
		}
	}
	if len(fields) != len(WorkerResultRequiredFields()) {
		return "", "", false
	}

	var parsed WorkerResult
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", "", false
	}
	if parsed.SchemaVersion != 1 || !meaningfulWorkerResultText(parsed.Summary) || !meaningfulWorkerResultText(parsed.Verdict) {
		return "", "", false
	}
	if !validStringList(parsed.Findings) || !validStringList(parsed.Risks) || !validStringList(parsed.NextActions) {
		return "", "", false
	}
	if !ValidWorkerResultConfidence(parsed.Confidence) {
		return "", "", false
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
	return strings.TrimSpace(b.String()), string(normalised), true
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
