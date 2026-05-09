package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Client handles communication with OpenAI-compatible HTTP endpoints.
type Client struct {
	baseURL           string
	apiKey            string
	oauthTokenURL     string
	oauthClientID     string
	oauthClientSecret string
	httpClient        *http.Client

	tokenMu        sync.Mutex
	cachedToken    string
	tokenExpiresAt time.Time
}

// NewClient creates a new Client.
func NewClient(baseURL, apiKeyEnv string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL()
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var apiKey string
	if apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
	}

	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultBaseURL returns the default local base URL, typically Ollama.
func DefaultBaseURL() string {
	if url := os.Getenv("OPENAI_COMPATIBLE_BASE_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:11434/v1"
}

// SetAPIKey sets the literal API key.
func (c *Client) SetAPIKey(key string) {
	if key != "" {
		c.apiKey = key
	}
}

// SetOAuthCredentials configures 2-legged OAuth client credentials.
func (c *Client) SetOAuthCredentials(tokenURL, clientID, clientSecret string) {
	c.oauthTokenURL = tokenURL
	c.oauthClientID = clientID
	c.oauthClientSecret = clientSecret
}

// getBearerToken resolves the authentication token for requests.
// It prioritizes a static API key. If absent, it attempts to resolve an OAuth token.
func (c *Client) getBearerToken(ctx context.Context) (string, error) {
	if c.apiKey != "" {
		return c.apiKey, nil
	}
	if c.oauthTokenURL == "" || c.oauthClientID == "" || c.oauthClientSecret == "" {
		return "", nil // No auth configured
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Add a 5-second buffer before actual expiration
	if c.cachedToken != "" && time.Now().Add(5*time.Second).Before(c.tokenExpiresAt) {
		return c.cachedToken, nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.oauthClientID)
	data.Set("client_secret", c.oauthClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("oauth token request returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // Usually in seconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode oauth token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("oauth token response contained no access token")
	}

	c.cachedToken = tokenResp.AccessToken
	expiresIn := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 1 * time.Hour // Fallback if missing
	}
	c.tokenExpiresAt = time.Now().Add(expiresIn)

	return c.cachedToken, nil
}

// CreateChatCompletion sends a chat completion request to the server.
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	var resp ChatCompletionResponse
	if err := c.postJSON(ctx, "/chat/completions", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return nil, fmt.Errorf("api error: %s", resp.Error.Message)
	}
	return &resp, nil
}

// StreamChatCompletion sends a streaming chat completion request and yields responses.
func (c *Client) StreamChatCompletion(ctx context.Context, req ChatCompletionRequest) (<-chan *ChatCompletionResponse, <-chan error, error) {
	req.Stream = true

	data, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	token, err := c.getBearerToken(ctx)
	if err != nil {
		return nil, nil, err
	}
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("http request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var apiErr Error
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return nil, nil, fmt.Errorf("api error %d: %s", resp.StatusCode, apiErr.Message)
		}
		return nil, nil, fmt.Errorf("http error %d: %s", resp.StatusCode, string(body))
	}

	responses := make(chan *ChatCompletionResponse, 100)
	errors := make(chan error, 1)

	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(responses)
		defer close(errors)

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					errors <- err
				}
				return
			}

			lineStr := strings.TrimSpace(string(line))
			if lineStr == "" {
				continue
			}

			if !strings.HasPrefix(lineStr, "data: ") {
				continue
			}

			dataStr := strings.TrimPrefix(lineStr, "data: ")
			if dataStr == "[DONE]" {
				return
			}

			var streamResp ChatCompletionResponse
			if err := json.Unmarshal([]byte(dataStr), &streamResp); err != nil {
				errors <- fmt.Errorf("failed to unmarshal stream chunk: %w", err)
				return
			}

			select {
			case <-ctx.Done():
				return
			case responses <- &streamResp:
			}
		}
	}()

	return responses, errors, nil
}

// CreateEmbeddings sends an embeddings request to the server.
func (c *Client) CreateEmbeddings(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	var resp EmbeddingResponse
	if err := c.postJSON(ctx, "/embeddings", req, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return nil, fmt.Errorf("api error: %s", resp.Error.Message)
	}
	return &resp, nil
}

// ListModels sends a GET request to the /v1/models endpoint.
func (c *Client) ListModels(ctx context.Context) (*ModelListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	token, err := c.getBearerToken(ctx)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr Error
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, apiErr.Message)
		}
		return nil, fmt.Errorf("http error %d: %s", resp.StatusCode, string(body))
	}

	var listResp ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &listResp, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, responseTarget any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	token, err := c.getBearerToken(ctx)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr Error
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return fmt.Errorf("api error %d: %s", resp.StatusCode, apiErr.Message)
		}
		return fmt.Errorf("http error %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(responseTarget); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
