package controlplane

import (
	"encoding/json"
	"fmt"
	"strings"
)

func GenerateTextPrompt(input GenerateTextInput) string {
	if len(input.Messages) == 0 {
		return strings.TrimSpace(input.Prompt)
	}
	var b strings.Builder
	for _, message := range input.Messages {
		content := messageContentText(message.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if strings.TrimSpace(message.Role) != "" {
			b.WriteString(strings.TrimSpace(message.Role))
			b.WriteString(": ")
		}
		b.WriteString(content)
	}
	if strings.TrimSpace(input.Prompt) != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(input.Prompt))
	}
	return strings.TrimSpace(b.String())
}

func messageContentText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}
