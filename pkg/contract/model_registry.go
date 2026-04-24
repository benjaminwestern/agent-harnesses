package contract

type ModelAlias struct {
	Alias string `json:"alias"`
	Model string `json:"model"`
}

type RuntimeProviderRegistry struct {
	Provider     string         `json:"provider"`
	DisplayName  string         `json:"display_name,omitempty"`
	DefaultModel string         `json:"default_model,omitempty"`
	Models       []RuntimeModel `json:"models,omitempty"`
}

type RuntimeBackendRegistry struct {
	Backend         string                    `json:"backend"`
	DisplayName     string                    `json:"display_name,omitempty"`
	Installed       bool                      `json:"installed"`
	SupportsSession bool                      `json:"supports_session"`
	ModelSource     string                    `json:"model_source,omitempty"`
	DefaultModel    string                    `json:"default_model,omitempty"`
	DefaultProvider string                    `json:"default_provider,omitempty"`
	Aliases         []ModelAlias              `json:"aliases,omitempty"`
	Providers       []RuntimeProviderRegistry `json:"providers,omitempty"`
	Models          []RuntimeModel            `json:"models,omitempty"`
	Issues          []string                  `json:"issues,omitempty"`
}

type ModelRegistry struct {
	SchemaVersion string                   `json:"schema_version"`
	Backends      []RuntimeBackendRegistry `json:"backends,omitempty"`
}
