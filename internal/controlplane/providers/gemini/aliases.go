package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

const geminiSettingsEnv = "GEMINI_CLI_SYSTEM_SETTINGS_PATH"

var (
	gemini3Pattern  = regexp.MustCompile(`(?i)^(?:auto-)?gemini-3(?:[.-]|$)`)
	gemini25Pattern = regexp.MustCompile(`(?i)^(?:auto-)?gemini-2\.5(?:[.-]|$)`)
)

type geminiModelAlias struct {
	Model        string
	Alias        string
	SettingsPath string
}

func prepareGeminiModelAlias(sessionID string, model string, options api.ModelOptions) (geminiModelAlias, error) {
	alias, config := getGeminiThinkingModelAlias(model, options)
	if alias == "" {
		return geminiModelAlias{Model: model}, nil
	}

	path, err := writeGeminiSystemSettings(sessionID, map[string]any{alias: config})
	if err != nil {
		return geminiModelAlias{}, err
	}
	return geminiModelAlias{
		Model:        model,
		Alias:        alias,
		SettingsPath: path,
	}, nil
}

func getGeminiThinkingModelAlias(model string, options api.ModelOptions) (string, map[string]any) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", nil
	}

	base := sanitizeGeminiAliasSegment(model)
	switch {
	case isGemini3Model(model) && isGeminiThinkingLevel(options.ThinkingLevel):
		level := strings.ToUpper(strings.TrimSpace(options.ThinkingLevel))
		return fmt.Sprintf("agentic-gemini-%s-thinking-level-%s", base, strings.ToLower(level)), map[string]any{
			"extends": "chat-base-3",
			"modelConfig": map[string]any{
				"model": model,
				"generateContentConfig": map[string]any{
					"thinkingConfig": map[string]any{
						"thinkingLevel": level,
					},
				},
			},
		}
	case isGemini25Model(model) && options.ThinkingBudget != nil && isGeminiThinkingBudget(*options.ThinkingBudget):
		budget := *options.ThinkingBudget
		label := fmt.Sprintf("%d", budget)
		if budget == -1 {
			label = "dynamic"
		}
		return fmt.Sprintf("agentic-gemini-%s-thinking-budget-%s", base, label), map[string]any{
			"extends": "chat-base-2.5",
			"modelConfig": map[string]any{
				"model": model,
				"generateContentConfig": map[string]any{
					"thinkingConfig": map[string]any{
						"thinkingBudget": budget,
					},
				},
			},
		}
	default:
		return "", nil
	}
}

func writeGeminiSystemSettings(sessionID string, aliases map[string]any) (string, error) {
	dir := filepath.Join(os.TempDir(), "agentic-control", "gemini")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "session"
	}

	path := filepath.Join(dir, fmt.Sprintf("%s-%d-%d.json", sanitizeGeminiAliasSegment(sessionID), os.Getpid(), time.Now().UnixNano()))
	settings := map[string]any{
		"modelConfigs": map[string]any{
			"aliases": aliases,
		},
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func isGemini3Model(model string) bool {
	return gemini3Pattern.MatchString(strings.TrimSpace(model))
}

func isGemini25Model(model string) bool {
	return gemini25Pattern.MatchString(strings.TrimSpace(model))
}

func isGeminiThinkingLevel(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOW", "HIGH":
		return true
	default:
		return false
	}
}

func isGeminiThinkingBudget(value int) bool {
	switch value {
	case -1, 0, 512:
		return true
	default:
		return false
	}
}

func sanitizeGeminiAliasSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNumeric := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNumeric {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(builder.String(), "-")
	if segment == "" {
		return "model"
	}
	return segment
}
