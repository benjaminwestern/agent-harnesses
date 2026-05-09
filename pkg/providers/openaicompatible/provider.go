package openaicompatible

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/config"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/httpclient/openaicompat"
)

const runtimeName = "openai-compatible"

type eventSink func(contract.RuntimeEvent)

type Provider struct {
	mu              sync.RWMutex
	sessions        map[string]*session
	emit            eventSink
	configModels    []contract.RuntimeModel
	configEndpoints []config.OpenAICompatibleEndpoint
}

type session struct {
	mu           sync.RWMutex
	appSessionID string
	cwd          string
	model        string
	baseURL      string
	apiKey       string
	modelOptions api.ModelOptions
	createdAtMS  int64
	updatedAtMS  int64
	status       contract.SessionStatus
	lastError    string
	messages     []openaicompat.ChatMessage
	provider     *Provider
}

func resolveAPIKey(ep config.OpenAICompatibleEndpoint) string {
	if ep.APIKeyEnv != "" {
		if val := os.Getenv(ep.APIKeyEnv); val != "" {
			return val
		}
	}
	return ep.APIKey
}

func NewProvider(emit func(contract.RuntimeEvent), cfg config.RuntimeConfig) *Provider {
	var models []contract.RuntimeModel
	for _, ep := range cfg.Endpoints {
		for _, m := range ep.Models {
			models = append(models, contract.RuntimeModel{
				ID:       m,
				Label:    m,
				Provider: ep.Provider,
				Custom:   true,
				DefaultOptions: map[string]any{
					"base_url":            ep.BaseURL,
					"api_key":             resolveAPIKey(ep),
					"oauth_token_url":     ep.OAuthTokenURL,
					"oauth_client_id":     ep.OAuthClientID,
					"oauth_client_secret": ep.OAuthClientSecret,
				},
			})
		}
	}
	for _, customModel := range cfg.Models {
		models = append(models, contract.RuntimeModel{
			ID:             customModel.ID,
			Label:          customModel.Label,
			Provider:       customModel.Provider,
			Custom:         true,
			DefaultOptions: customModel.Options,
		})
	}

	return &Provider{
		sessions:        make(map[string]*session),
		emit:            emit,
		configModels:    models,
		configEndpoints: append([]config.OpenAICompatibleEndpoint(nil), cfg.Endpoints...),
	}
}

func (p *Provider) Runtime() string {
	return runtimeName
}

func (p *Provider) Describe() contract.RuntimeDescriptor {
	descriptor := contract.NewRuntimeDescriptor(
		runtimeName,
		contract.OwnershipControlled,
		contract.TransportAppServer,
		contract.RuntimeCapabilities{
			StartSession:             true,
			ResumeSession:            true,
			SendInput:                true,
			Interrupt:                true,
			Respond:                  false,
			StopSession:              true,
			ListSessions:             true,
			StreamEvents:             true,
			ApprovalRequests:         false,
			UserInputRequests:        false,
			ImmediateProviderSession: true,
			ResumeByProviderID:       true,
			TextGeneration:           true,
			Embeddings:               true,
		},
	)

	installed := false
	var models []contract.RuntimeModel

	p.mu.RLock()
	models = append(models, p.configModels...)
	endpoints := append([]config.OpenAICompatibleEndpoint(nil), p.configEndpoints...)
	p.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if discovered := listOpenAICompatibleModels(ctx, openaicompat.NewClient("", ""), "ollama"); len(discovered) > 0 {
		installed = true
		models = append(models, discovered...)
	}
	for _, endpoint := range endpoints {
		client := openAIClientForOptions(api.ModelOptions{
			BaseURL:           endpoint.BaseURL,
			APIKey:            resolveAPIKey(endpoint),
			OAuthTokenURL:     endpoint.OAuthTokenURL,
			OAuthClientID:     endpoint.OAuthClientID,
			OAuthClientSecret: endpoint.OAuthClientSecret,
		})
		if discovered := listOpenAICompatibleModels(ctx, client, endpoint.Provider); len(discovered) > 0 {
			installed = true
			models = append(models, discovered...)
		}
	}

	descriptor.Probe = &contract.RuntimeProbe{
		Installed:   installed || len(models) > 0,
		ModelSource: "dynamic",
		Models:      models,
	}

	return descriptor
}

func listOpenAICompatibleModels(ctx context.Context, client *openaicompat.Client, provider string) []contract.RuntimeModel {
	listResp, err := client.ListModels(ctx)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(provider) == "" {
		provider = "openai-compatible"
	}
	models := make([]contract.RuntimeModel, 0, len(listResp.Data))
	for _, m := range listResp.Data {
		models = append(models, contract.RuntimeModel{
			ID:       m.ID,
			Label:    m.ID,
			Provider: provider,
			Custom:   true,
		})
	}
	return models
}

func (p *Provider) StartSession(ctx context.Context, request api.StartSessionRequest) (*contract.RuntimeSession, error) {
	now := time.Now().UnixMilli()

	var messages []openaicompat.ChatMessage
	if systemPrompt, ok := request.Metadata["system_prompt"].(string); ok && systemPrompt != "" {
		messages = append(messages, openaicompat.ChatMessage{Role: "system", Content: systemPrompt})
	}

	sess := &session{
		appSessionID: request.SessionID,
		cwd:          request.CWD,
		model:        request.Model,
		baseURL:      request.ModelOptions.BaseURL,
		apiKey:       request.ModelOptions.APIKey,
		modelOptions: request.ModelOptions,
		createdAtMS:  now,
		updatedAtMS:  now,
		status:       contract.SessionIdle,
		messages:     messages,
		provider:     p,
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "session/start", "", "Started OpenAI-compatible session", map[string]any{"status": string(contract.SessionIdle)}))

	if request.Prompt != "" {
		if _, err := p.SendInput(ctx, api.SendInputRequest{
			SessionID: request.SessionID,
			Text:      request.Prompt,
			Metadata:  request.Metadata,
		}); err != nil {
			return nil, err
		}
	}
	return sess.snapshot(), nil
}

func (p *Provider) ResumeSession(ctx context.Context, request api.ResumeSessionRequest) (*contract.RuntimeSession, error) {
	now := time.Now().UnixMilli()
	sess := &session{
		appSessionID: request.SessionID,
		cwd:          request.CWD,
		model:        request.Model,
		baseURL:      request.ModelOptions.BaseURL,
		apiKey:       request.ModelOptions.APIKey,
		modelOptions: request.ModelOptions,
		createdAtMS:  now,
		updatedAtMS:  now,
		status:       contract.SessionIdle,
		provider:     p,
	}

	p.mu.Lock()
	p.sessions[request.SessionID] = sess
	p.mu.Unlock()

	p.emit(p.newEvent(sess, "session.started", "session/resume", "", "Resumed OpenAI-compatible session", map[string]any{"status": string(contract.SessionIdle)}))

	return sess.snapshot(), nil
}

func (p *Provider) SendInput(ctx context.Context, request api.SendInputRequest) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(request.SessionID)
	if err != nil {
		return nil, err
	}
	if err := contract.ValidateContentParts(request.Parts); err != nil {
		return nil, err
	}

	var content any = request.Text
	if len(request.Parts) > 0 {
		var parts []openaicompat.ChatContentPart
		if request.Text != "" {
			parts = append(parts, openaicompat.ChatContentPart{
				Type: contract.ContentPartTypeText,
				Text: request.Text,
			})
		}
		for _, part := range request.Parts {
			switch part.Type {
			case contract.ContentPartTypeText:
				parts = append(parts, openaicompat.ChatContentPart{
					Type: contract.ContentPartTypeText,
					Text: part.Text,
				})
			case contract.ContentPartTypeImage:
				url := part.URL
				if url == "" && part.Data != "" {
					mime := part.MIMEType
					if mime == "" {
						mime = "image/jpeg"
					}
					url = fmt.Sprintf("data:%s;base64,%s", mime, part.Data)
				}
				parts = append(parts, openaicompat.ChatContentPart{
					Type: "image_url",
					ImageURL: &openaicompat.ChatImageURL{
						URL: url,
					},
				})
			default:
				return nil, fmt.Errorf("openai-compatible content part type %q is not supported", part.Type)
			}
		}
		if len(parts) > 0 {
			content = parts
		}
	}

	sess.mu.Lock()
	sess.messages = append(sess.messages, openaicompat.ChatMessage{Role: "user", Content: content})
	sess.status = contract.SessionRunning
	sess.updatedAtMS = time.Now().UnixMilli()
	sess.mu.Unlock()

	turnID := fmt.Sprintf("turn-%d", time.Now().UnixNano())

	startPayload := map[string]any{"status": string(contract.SessionRunning)}
	if request.Text != "" {
		startPayload[contract.PayloadInputText] = request.Text
	}
	if len(request.Parts) > 0 {
		startPayload[contract.PayloadInputParts] = request.Parts
	}
	event := p.newEvent(sess, "turn.started", "turn/start", turnID, fmt.Sprintf("Started turn: %s", request.Text), startPayload)
	p.emit(event)

	// In background, call OpenAI compatible API
	go func() {
		sess.mu.RLock()
		model := sess.model
		messages := append([]openaicompat.ChatMessage(nil), sess.messages...)
		baseURL := sess.baseURL
		apiKey := sess.apiKey
		sess.mu.RUnlock()

		if model == "" {
			model = "ollama" // fallback
		}

		client := openAIClientForOptions(api.ModelOptions{
			BaseURL:           baseURL,
			APIKey:            apiKey,
			OAuthTokenURL:     sess.modelOptions.OAuthTokenURL,
			OAuthClientID:     sess.modelOptions.OAuthClientID,
			OAuthClientSecret: sess.modelOptions.OAuthClientSecret,
		})

		req := openaicompat.ChatCompletionRequest{
			Model:           model,
			Messages:        messages,
			ReasoningEffort: sess.modelOptions.ReasoningEffort,
			Logprobs:        sess.modelOptions.Logprobs,
			TopLogprobs:     sess.modelOptions.TopLogprobs,
		}

		if sess.modelOptions.ResponseSchema != nil {
			req.ResponseFormat = &openaicompat.ResponseFormat{
				Type: "json_schema",
				JSONSchema: &openaicompat.JSONSchemaDef{
					Name:   "structured_output",
					Strict: true,
					Schema: sess.modelOptions.ResponseSchema,
				},
			}
		}

		responses, errs, err := client.StreamChatCompletion(context.Background(), req)
		if err != nil {
			sess.mu.Lock()
			sess.status = contract.SessionErrored
			sess.lastError = err.Error()
			sess.updatedAtMS = time.Now().UnixMilli()
			sess.mu.Unlock()
			p.emit(p.newEvent(sess, "session.errored", "turn/error", turnID, fmt.Sprintf("Turn failed: %v", err), map[string]any{"status": string(contract.SessionErrored), "last_error": err.Error()}))
			return
		}

		var accumulatedText string
		var accumulatedLogprobs []openaicompat.TokenLogprob

		for {
			select {
			case streamErr, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if streamErr != nil {
					sess.mu.Lock()
					sess.status = contract.SessionErrored
					sess.lastError = streamErr.Error()
					sess.updatedAtMS = time.Now().UnixMilli()
					sess.mu.Unlock()
					p.emit(p.newEvent(sess, "session.errored", "turn/error", turnID, fmt.Sprintf("Stream failed: %v", streamErr), map[string]any{"status": string(contract.SessionErrored), "last_error": streamErr.Error()}))
					return
				}
			case resp, ok := <-responses:
				if !ok {
					// Stream complete
					finalMsg := openaicompat.ChatMessage{
						Role:    "assistant",
						Content: accumulatedText,
					}

					sess.mu.Lock()
					sess.messages = append(sess.messages, finalMsg)
					sess.status = contract.SessionIdle
					sess.updatedAtMS = time.Now().UnixMilli()
					sess.mu.Unlock()

					payload := map[string]any{"status": string(contract.SessionIdle)}
					if len(accumulatedLogprobs) > 0 {
						payload["logprobs"] = accumulatedLogprobs
					}
					p.emit(p.newEvent(sess, "turn.completed", "turn/complete", turnID, "Turn completed", payload))
					return
				}

				if len(resp.Choices) > 0 {
					delta := openaicompat.ChoiceContentText(resp.Choices[0])
					if delta != "" {
						accumulatedText += delta
						p.emit(p.newEvent(sess, "assistant.message.delta", "message/delta", turnID, delta, map[string]any{"delta": delta}))
					}
					if resp.Choices[0].Logprobs != nil && len(resp.Choices[0].Logprobs.Content) > 0 {
						accumulatedLogprobs = append(accumulatedLogprobs, resp.Choices[0].Logprobs.Content...)
					}
				}
			}
		}
	}()
	return &event, nil
}

func (p *Provider) Interrupt(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	sess.status = contract.SessionInterrupted
	sess.updatedAtMS = time.Now().UnixMilli()
	sess.mu.Unlock()

	event := p.newEvent(sess, "turn.interrupted", "turn/interrupt", "", "Interrupted turn", map[string]any{"status": string(contract.SessionInterrupted)})
	p.emit(event)
	return &event, nil
}

func (p *Provider) Respond(ctx context.Context, request api.RespondRequest) (*contract.RuntimeEvent, error) {
	return nil, fmt.Errorf("respond is not supported on the openai-compatible provider")
}

func (p *Provider) StopSession(ctx context.Context, sessionID string) (*contract.RuntimeEvent, error) {
	sess, err := p.getSession(sessionID)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	sess.status = contract.SessionStopped
	sess.updatedAtMS = time.Now().UnixMilli()
	sess.mu.Unlock()

	p.deleteSession(sessionID)
	event := p.newEvent(sess, "session.stopped", "session/stop", "", "Stopped session", map[string]any{"status": string(contract.SessionStopped)})
	p.emit(event)
	return &event, nil
}

func (p *Provider) GenerateEmbeddings(ctx context.Context, input api.EmbeddingInput) (*api.EmbeddingOutput, error) {
	model := input.ModelSelection.Model
	if model == "" {
		model = "nomic-embed-text"
	}

	baseURL := input.ModelSelection.Options.BaseURL
	apiKey := input.ModelSelection.Options.APIKey

	client := openAIClientForOptions(api.ModelOptions{
		BaseURL:           baseURL,
		APIKey:            apiKey,
		OAuthTokenURL:     input.ModelSelection.Options.OAuthTokenURL,
		OAuthClientID:     input.ModelSelection.Options.OAuthClientID,
		OAuthClientSecret: input.ModelSelection.Options.OAuthClientSecret,
	})

	resp, err := client.CreateEmbeddings(ctx, openaicompat.EmbeddingRequest{
		Model: model,
		Input: input.Texts,
	})

	if err != nil {
		return nil, fmt.Errorf("openai-compatible embedding failed: %w", err)
	}

	vectors := make([][]float64, 0, len(resp.Data))
	for _, item := range resp.Data {
		vectors = append(vectors, item.Embedding)
	}

	return &api.EmbeddingOutput{
		Vectors:  vectors,
		Metadata: map[string]any{"model": resp.Model, "usage": resp.Usage},
	}, nil
}

func (p *Provider) ListSessions(ctx context.Context) ([]contract.RuntimeSession, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sessions := make([]contract.RuntimeSession, 0, len(p.sessions))
	for _, sess := range p.sessions {
		sessions = append(sessions, *sess.snapshot())
	}
	return sessions, nil
}

func (p *Provider) getSession(sessionID string) (*session, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sess, ok := p.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("unknown session: %s", sessionID)
	}
	return sess, nil
}

func (p *Provider) deleteSession(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, sessionID)
}

func (p *Provider) newEvent(sess *session, eventType string, nativeEventName string, turnID string, summary string, payload map[string]any) contract.RuntimeEvent {
	return contract.NewRuntimeEvent(*sess.snapshot(), eventType, nativeEventName, turnID, summary, payload)
}

func (s *session) snapshot() *contract.RuntimeSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &contract.RuntimeSession{
		SchemaVersion:    contract.ControlPlaneSchemaVersion,
		SessionID:        s.appSessionID,
		Runtime:          runtimeName,
		Ownership:        contract.OwnershipControlled,
		Transport:        contract.TransportAppServer,
		Status:           s.status,
		CWD:              s.cwd,
		Model:            s.model,
		CreatedAtMS:      s.createdAtMS,
		UpdatedAtMS:      s.updatedAtMS,
		LastActivityAtMS: s.updatedAtMS,
		LastError:        s.lastError,
	}
}
