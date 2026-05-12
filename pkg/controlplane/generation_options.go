package controlplane

import (
	"encoding/base64"
	"strings"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TextGenerationSelectionFromOptions(opts GenerateOptions) TextGenerationModelSelection {
	options := ModelOptions{
		BaseURL:         strings.TrimSpace(opts.BaseURL),
		APIKeyEnv:       strings.TrimSpace(opts.APIKeyEnv),
		APIKey:          strings.TrimSpace(opts.APIKey),
		MaxOutputTokens: opts.MaxOutputTokens,
	}
	if opts.Temperature > 0 {
		value := opts.Temperature
		options.Temperature = &value
	}
	if opts.TopP > 0 {
		value := opts.TopP
		options.TopP = &value
	}
	return TextGenerationModelSelection{
		Provider: strings.TrimSpace(opts.Provider),
		Model:    strings.TrimSpace(opts.Model),
		Options:  options,
	}
}

func GenerateTextInputWithMedia(input GenerateTextInput) GenerateTextInput {
	if len(input.Media) == 0 {
		return input
	}
	parts := ContentPartsFromMedia(input.Media)
	input.Media = nil
	if len(parts) == 0 {
		return input
	}

	if len(input.Messages) == 0 {
		messages := make([]Message, 0, 2)
		if strings.TrimSpace(input.SystemPrompt) != "" {
			messages = append(messages, Message{Role: "system", Content: input.SystemPrompt})
		}
		messages = append(messages, Message{
			Role:    "user",
			Content: input.Prompt,
			Parts:   parts,
		})
		input.Messages = messages
		input.Prompt = ""
		input.SystemPrompt = ""
		return input
	}

	userIndex := -1
	for i := len(input.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(input.Messages[i].Role), "user") {
			userIndex = i
			break
		}
	}
	if userIndex < 0 {
		input.Messages = append(input.Messages, Message{
			Role:    "user",
			Content: input.Prompt,
			Parts:   parts,
		})
		input.Prompt = ""
		return input
	}
	if input.Messages[userIndex].Content == nil && strings.TrimSpace(input.Prompt) != "" {
		input.Messages[userIndex].Content = input.Prompt
		input.Prompt = ""
	}
	input.Messages[userIndex].Parts = append(input.Messages[userIndex].Parts, parts...)
	return input
}

func ContentPartsFromMedia(media []MediaAttachment) []contract.ContentPart {
	parts := make([]contract.ContentPart, 0, len(media))
	for _, item := range media {
		if len(item.Data) == 0 {
			continue
		}
		mimeType := strings.ToLower(strings.TrimSpace(item.MimeType))
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
		if !strings.HasPrefix(mimeType, "image/") {
			continue
		}
		parts = append(parts, contract.ContentPart{
			Type:     contract.ContentPartTypeImage,
			MIMEType: mimeType,
			Data:     base64.StdEncoding.EncodeToString(item.Data),
		})
	}
	return parts
}
