package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

func (p *Provider) GenerateText(ctx context.Context, input api.GenerateTextInput) (*api.GenerateTextOutput, error) {
	prompt := api.GenerateTextPrompt(input)
	if input.SystemPrompt != "" {
		prompt = fmt.Sprintf("System Instructions: %s\n\n%s", input.SystemPrompt, prompt)
	}

	args := []string{"exec"}
	if input.ModelSelection.Model != "" {
		args = append(args, "-m", input.ModelSelection.Model)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, codexBinaryPath(), args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("codex text generation failed: %w\nstderr: %s", err, errOut.String())
	}

	text := strings.TrimSpace(out.String())
	if input.ResponseFormat == "json" || input.ResponseFormat == "json_object" {
		text = strings.TrimPrefix(text, "```json\n")
		text = strings.TrimSuffix(text, "\n```")
		text = strings.TrimSpace(text)
	}

	return &api.GenerateTextOutput{
		Text:     text,
		Metadata: map[string]any{},
	}, nil
}

func (p *Provider) GenerateCommitMessage(ctx context.Context, input api.CommitMessageInput) (*api.CommitMessageOutput, error) {
	prompt := fmt.Sprintf("Diff:\n%s", input.Diff)
	sys := "You are an expert software engineer. Write a concise, high-quality commit message for the provided diff. Follow conventional commits format."
	if input.Instruction != "" {
		sys = input.Instruction
	}
	out, err := p.GenerateText(ctx, api.GenerateTextInput{
		ModelSelection: input.ModelSelection,
		Prompt:         prompt,
		SystemPrompt:   sys,
		Metadata:       input.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return &api.CommitMessageOutput{Message: out.Text}, nil
}

func (p *Provider) GeneratePrContent(ctx context.Context, input api.PrContentInput) (*api.PrContentOutput, error) {
	prompt := fmt.Sprintf("Title: %s\n\nDiff:\n%s", input.Title, input.Diff)
	sys := "You are an expert software engineer. Write a Pull Request title and body based on the provided diff. Output MUST be a JSON object with 'title' and 'body' string fields."
	if input.Instruction != "" {
		sys = input.Instruction
	}
	out, err := p.GenerateText(ctx, api.GenerateTextInput{
		ModelSelection: input.ModelSelection,
		Prompt:         prompt,
		SystemPrompt:   sys,
		ResponseFormat: "json",
		Metadata:       input.Metadata,
	})
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out.Text), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse json response: %w\nRaw: %s", err, out.Text)
	}

	return &api.PrContentOutput{
		Title: parsed.Title,
		Body:  parsed.Body,
	}, nil
}

func (p *Provider) GenerateBranchName(ctx context.Context, input api.BranchNameInput) (*api.BranchNameOutput, error) {
	out, err := p.GenerateText(ctx, api.GenerateTextInput{
		ModelSelection: input.ModelSelection,
		Prompt:         input.Summary,
		SystemPrompt:   "Generate a short, valid git branch name based on the following summary. Output ONLY the branch name, lowercase with hyphens, no explanation.",
		Metadata:       input.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return &api.BranchNameOutput{Name: out.Text}, nil
}

func (p *Provider) GenerateThreadTitle(ctx context.Context, input api.ThreadTitleInput) (*api.ThreadTitleOutput, error) {
	out, err := p.GenerateText(ctx, api.GenerateTextInput{
		ModelSelection: input.ModelSelection,
		Prompt:         input.Prompt,
		SystemPrompt:   "Generate a short (3-6 words) descriptive title for a chat thread based on this initial prompt. Output ONLY the title, no explanation or quotes.",
		Metadata:       input.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return &api.ThreadTitleOutput{Title: out.Text}, nil
}
