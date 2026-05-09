package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Runtimes map[string]RuntimeConfig `toml:"runtimes"`
}

type RuntimeConfig struct {
	Models    []CustomModelConfig        `toml:"models"`
	Endpoints []OpenAICompatibleEndpoint `toml:"endpoints"`
}

type CustomModelConfig struct {
	ID       string         `toml:"id"`
	Label    string         `toml:"label"`
	Provider string         `toml:"provider"`
	Options  map[string]any `toml:"options"`
}

type OpenAICompatibleEndpoint struct {
	Provider          string   `toml:"provider"`
	BaseURL           string   `toml:"base_url"`
	APIKeyEnv         string   `toml:"api_key_env"`
	APIKey            string   `toml:"api_key"`
	Models            []string `toml:"models"`
	OAuthTokenURL     string   `toml:"oauth_token_url"`
	OAuthClientID     string   `toml:"oauth_client_id"`
	OAuthClientSecret string   `toml:"oauth_client_secret"`
}

func Load() Config {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}
	}
	path := filepath.Join(configDir, "agentic-control", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}
	var config Config
	if _, err := toml.Decode(string(data), &config); err != nil {
		return Config{}
	}
	return config
}

func (c *Config) Save() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dirPath := filepath.Join(configDir, "agentic-control")
	if err := os.MkdirAll(dirPath, 0700); err != nil {
		return err
	}
	path := filepath.Join(dirPath, "config.toml")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
