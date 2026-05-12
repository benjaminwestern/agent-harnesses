package controlplane

import (
	"context"
	"testing"
)

type captureTextProvider struct {
	input GenerateTextInput
}

func (p *captureTextProvider) GenerateCommitMessage(context.Context, CommitMessageInput) (*CommitMessageOutput, error) {
	return &CommitMessageOutput{}, nil
}

func (p *captureTextProvider) GeneratePrContent(context.Context, PrContentInput) (*PrContentOutput, error) {
	return &PrContentOutput{}, nil
}

func (p *captureTextProvider) GenerateBranchName(context.Context, BranchNameInput) (*BranchNameOutput, error) {
	return &BranchNameOutput{}, nil
}

func (p *captureTextProvider) GenerateThreadTitle(context.Context, ThreadTitleInput) (*ThreadTitleOutput, error) {
	return &ThreadTitleOutput{}, nil
}

func (p *captureTextProvider) GenerateText(_ context.Context, input GenerateTextInput) (*GenerateTextOutput, error) {
	p.input = input
	return &GenerateTextOutput{Text: "ok"}, nil
}

func TestGenerateTextWithOptionsBuildsSelection(t *testing.T) {
	provider := &captureTextProvider{}
	router := NewTextGenerationRouter("fixture", map[string]TextGenerationProvider{"fixture": provider})

	out, err := router.GenerateTextWithOptions(context.Background(), GenerateOptions{
		Provider:        "fixture",
		Model:           "model-fixture",
		BaseURL:         "http://example.test/v1",
		APIKeyEnv:       "OPENAI_API_KEY",
		APIKey:          "literal-key",
		SystemPrompt:    "system",
		MaxOutputTokens: 128,
		Temperature:     0.2,
		TopP:            0.8,
		ResponseFormat:  "json",
		Metadata:        map[string]any{"trace": "abc"},
	}, []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("GenerateTextWithOptions: %v", err)
	}
	if out.Text != "ok" {
		t.Fatalf("out.Text = %q, want ok", out.Text)
	}
	got := provider.input
	if got.ModelSelection.Provider != "fixture" || got.ModelSelection.Model != "model-fixture" {
		t.Fatalf("selection = %+v", got.ModelSelection)
	}
	options := got.ModelSelection.Options
	if options.BaseURL != "http://example.test/v1" || options.APIKeyEnv != "OPENAI_API_KEY" || options.APIKey != "literal-key" || options.MaxOutputTokens != 128 {
		t.Fatalf("options = %+v", options)
	}
	if options.Temperature == nil || *options.Temperature != 0.2 || options.TopP == nil || *options.TopP != 0.8 {
		t.Fatalf("sampling options = %+v", options)
	}
	if got.SystemPrompt != "system" || got.ResponseFormat != "json" || got.Metadata["trace"] != "abc" {
		t.Fatalf("input = %+v", got)
	}
}

func TestGenerateTextInputWithMediaAddsImageParts(t *testing.T) {
	input := GenerateTextInput{
		Prompt:       "describe",
		SystemPrompt: "be concise",
		Media: []MediaAttachment{
			{FileName: "image.png", MimeType: "image/png", Data: []byte("image")},
			{FileName: "notes.txt", MimeType: "text/plain", Data: []byte("skip")},
		},
	}
	got := GenerateTextInputWithMedia(input)
	if len(got.Media) != 0 {
		t.Fatalf("media was not consumed: %+v", got.Media)
	}
	if got.Prompt != "" || got.SystemPrompt != "" {
		t.Fatalf("prompt fields = %q/%q, want cleared after message construction", got.Prompt, got.SystemPrompt)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("message count = %d, want system + user", len(got.Messages))
	}
	user := got.Messages[1]
	if user.Role != "user" || user.Content != "describe" {
		t.Fatalf("user message = %+v", user)
	}
	if len(user.Parts) != 1 || user.Parts[0].Type != "image" || user.Parts[0].MIMEType != "image/png" || user.Parts[0].Data != "aW1hZ2U=" {
		t.Fatalf("media parts = %+v", user.Parts)
	}
}
