package openaicompatible

import (
	"net/http"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/httpclient/openaicompat"
)

type EndpointConfig struct {
	Provider          string
	BaseURL           string
	APIKeyEnv         string
	APIKey            string
	Models            []string
	OAuthTokenURL     string
	OAuthClientID     string
	OAuthClientSecret string
	Timeout           time.Duration
	HTTPClient        *http.Client
	RetryPolicy       openaicompat.RetryPolicy
}

type ModelConfig struct {
	ID       string
	Label    string
	Provider string
	Options  map[string]any
}

type ProviderConfig struct {
	Models    []ModelConfig
	Endpoints []EndpointConfig
}

func (cfg EndpointConfig) client() *openaicompat.Client {
	client := openaicompat.NewClientWithOptions(cfg.BaseURL, cfg.APIKeyEnv, openaicompat.ClientOptions{
		APIKey:            cfg.APIKey,
		OAuthTokenURL:     cfg.OAuthTokenURL,
		OAuthClientID:     cfg.OAuthClientID,
		OAuthClientSecret: cfg.OAuthClientSecret,
		HTTPClient:        cfg.HTTPClient,
		Timeout:           cfg.Timeout,
		RetryPolicy:       cfg.RetryPolicy,
	})
	return client
}
