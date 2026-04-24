package controlplane

import (
	"encoding/json"
	"strings"
)

type StructuredJSONParser func(candidate string) (rendered string, normalisedJSON string, ok bool)

func ExtractStructuredJSON(output string, parser StructuredJSONParser) (string, string) {
	var fallback string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if fallback == "" {
			fallback = line
		}
		for _, candidate := range JSONObjectCandidates(line) {
			rendered, normalised, ok := parser(candidate)
			if ok {
				fallback = rendered
				if normalised != "" {
					return rendered, normalised
				}
			}
		}
	}

	lines := strings.Split(output, "\n")
	for start := len(lines) - 1; start >= 0; start-- {
		suffix := strings.TrimSpace(strings.Join(lines[start:], "\n"))
		if suffix == "" {
			continue
		}
		if fallback == "" {
			fallback = suffix
		}
		for _, candidate := range JSONObjectCandidates(suffix) {
			rendered, normalised, ok := parser(candidate)
			if ok {
				return rendered, normalised
			}
		}
	}

	for _, candidate := range JSONObjectCandidates(output) {
		rendered, normalised, ok := parser(candidate)
		if ok {
			return rendered, normalised
		}
	}
	return strings.TrimSpace(firstNonEmptyValue(fallback, output)), ""
}

func JSONObjectCandidates(output string) []string {
	var out []string
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		out = append(out, trimmed)
	}
	for _, candidate := range balancedJSONObjectCandidates(output) {
		if !containsStringValue(out, candidate) {
			out = append(out, candidate)
		}
	}
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(output[start : end+1])
		if !containsStringValue(out, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func balancedJSONObjectCandidates(output string) []string {
	var out []string
	for start, r := range output {
		if r != '{' {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(output[start:]))
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			continue
		}
		candidate := strings.TrimSpace(string(raw))
		if strings.HasPrefix(candidate, "{") && strings.HasSuffix(candidate, "}") && !containsStringValue(out, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
