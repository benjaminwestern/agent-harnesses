package controlplane

const (
	MetadataKeyTitle    = "title"
	MetadataKeySystem   = "system"
	MetadataKeyAgent    = "agent"
	MetadataKeyModel    = "model"
	MetadataKeyProvider = "provider"
	MetadataKeyTools    = "tools"
)

var DefaultNoToolTurnTools = []string{
	"bash",
	"edit",
	"glob",
	"grep",
	"list",
	"patch",
	"read",
	"todowrite",
	"webfetch",
	"write",
}

type RuntimeMetadata struct {
	Title    string
	System   string
	Agent    string
	Model    string
	Provider string
	Labels   map[string]string
	Extra    map[string]any
}

func (metadata RuntimeMetadata) Map() map[string]any {
	out := make(map[string]any, len(metadata.Extra)+len(metadata.Labels)+5)
	for key, value := range metadata.Extra {
		if key != "" && value != nil {
			out[key] = value
		}
	}
	putString(out, MetadataKeyTitle, metadata.Title)
	putString(out, MetadataKeySystem, metadata.System)
	putString(out, MetadataKeyAgent, metadata.Agent)
	putString(out, MetadataKeyModel, metadata.Model)
	putString(out, MetadataKeyProvider, metadata.Provider)
	for key, value := range metadata.Labels {
		putString(out, key, value)
	}
	return out
}

func MetadataForNoToolTurn(metadata map[string]any) map[string]any {
	return MetadataWithDisabledTools(metadata, DefaultNoToolTurnTools...)
}

func MetadataWithDisabledTools(metadata map[string]any, tools ...string) map[string]any {
	next := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		next[key] = value
	}
	disabled := make(map[string]any, len(tools))
	for _, tool := range tools {
		if tool != "" {
			disabled[tool] = false
		}
	}
	next[MetadataKeyTools] = disabled
	return next
}

func putString(values map[string]any, key string, value string) {
	if key != "" && value != "" {
		values[key] = value
	}
}
