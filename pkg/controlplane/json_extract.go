package controlplane

import (
	"encoding/json"
	"strings"
)

type StructuredJSONParser func(candidate string) (rendered string, normalisedJSON string, err error)

func ExtractStructuredJSON(output string, parser StructuredJSONParser) (string, string, error) {
	var fallback string
	var lastErr error

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if fallback == "" {
			fallback = line
		}
		for _, candidate := range JSONObjectCandidates(line) {
			rendered, normalised, err := parser(candidate)
			if err == nil {
				if normalised != "" {
					return rendered, normalised, nil
				}
				fallback = rendered
			} else {
				lastErr = err
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
			rendered, normalised, err := parser(candidate)
			if err == nil {
				return rendered, normalised, nil
			} else {
				lastErr = err
			}
		}
	}

	for _, candidate := range JSONObjectCandidates(output) {
		rendered, normalised, err := parser(candidate)
		if err == nil {
			return rendered, normalised, nil
		} else {
			lastErr = err
		}
	}

	return strings.TrimSpace(firstNonEmptyValue(fallback, output)), "", lastErr
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
