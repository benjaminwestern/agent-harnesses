package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/benjaminwestern/agentic-control/internal/config"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup and configuration commands",
	}
	cmd.AddCommand(newSetupEndpointCmd())
	return cmd
}

func newSetupEndpointCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "endpoint",
		Short: "Interactively add a new OpenAI-compatible endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Print("Provider Name (e.g. groq, vllm, local-ollama): ")
			provider, _ := reader.ReadString('\n')
			provider = strings.TrimSpace(provider)
			if provider == "" {
				return fmt.Errorf("provider name is required")
			}

			fmt.Print("Base URL (e.g. https://api.groq.com/openai/v1): ")
			baseURL, _ := reader.ReadString('\n')
			baseURL = strings.TrimSpace(baseURL)
			if baseURL == "" {
				return fmt.Errorf("base URL is required")
			}

			fmt.Print("API Key (leave blank for none): ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			var apiKeyEnv string
			if apiKey == "" {
				fmt.Print("API Key Environment Variable (e.g. GROQ_API_KEY, leave blank for none): ")
				apiKeyEnv, _ = reader.ReadString('\n')
				apiKeyEnv = strings.TrimSpace(apiKeyEnv)
			}

			fmt.Print("Models to register (comma-separated, e.g. llama3-70b, mixtral-8x7b): ")
			modelsStr, _ := reader.ReadString('\n')
			modelsStr = strings.TrimSpace(modelsStr)
			var models []string
			if modelsStr != "" {
				for _, m := range strings.Split(modelsStr, ",") {
					m = strings.TrimSpace(m)
					if m != "" {
						models = append(models, m)
					}
				}
			}

			fmt.Print("OAuth 2.0 Token URL (leave blank if not using 2-legged OAuth): ")
			oauthTokenURL, _ := reader.ReadString('\n')
			oauthTokenURL = strings.TrimSpace(oauthTokenURL)

			var oauthClientID, oauthClientSecret string
			if oauthTokenURL != "" {
				fmt.Print("OAuth 2.0 Client ID: ")
				oauthClientID, _ = reader.ReadString('\n')
				oauthClientID = strings.TrimSpace(oauthClientID)

				fmt.Print("OAuth 2.0 Client Secret: ")
				oauthClientSecret, _ = reader.ReadString('\n')
				oauthClientSecret = strings.TrimSpace(oauthClientSecret)
			}

			cfg := config.Load()
			if cfg.Runtimes == nil {
				cfg.Runtimes = make(map[string]config.RuntimeConfig)
			}

			openaiCfg := cfg.Runtimes["openai-compatible"]

			// Check if provider already exists
			exists := false
			for i, ep := range openaiCfg.Endpoints {
				if ep.Provider == provider {
					openaiCfg.Endpoints[i] = config.OpenAICompatibleEndpoint{
						Provider:          provider,
						BaseURL:           baseURL,
						APIKey:            apiKey,
						APIKeyEnv:         apiKeyEnv,
						Models:            models,
						OAuthTokenURL:     oauthTokenURL,
						OAuthClientID:     oauthClientID,
						OAuthClientSecret: oauthClientSecret,
					}
					exists = true
					break
				}
			}

			if !exists {
				openaiCfg.Endpoints = append(openaiCfg.Endpoints, config.OpenAICompatibleEndpoint{
					Provider:          provider,
					BaseURL:           baseURL,
					APIKey:            apiKey,
					APIKeyEnv:         apiKeyEnv,
					Models:            models,
					OAuthTokenURL:     oauthTokenURL,
					OAuthClientID:     oauthClientID,
					OAuthClientSecret: oauthClientSecret,
				})
			}

			cfg.Runtimes["openai-compatible"] = openaiCfg

			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\nSuccessfully added endpoint '%s'!\n", provider)
			fmt.Println("Run 'agent_control describe' to verify it is registered.")
			return nil
		},
	}
}
