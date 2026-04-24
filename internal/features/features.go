package features

import (
	"os"
	"strings"
)

// Features defines the set of feature flags for the application.
type Features struct {
	// EnableVerboseLogging enables detailed debug information.
	EnableVerboseLogging bool
	// EnableParallelExecution enables parallel processing in workflows.
	EnableParallelExecution bool
	// UseExperimentalProvider enables a new, experimental provider backend.
	UseExperimentalProvider bool
}

// FromMap populates Features from a map of values, typically from a config file.
func FromMap(values map[string]any) Features {
	f := Features{
		EnableVerboseLogging:    boolValue(values["verbose_logging"]),
		EnableParallelExecution: boolValue(values["parallel_execution"]),
		UseExperimentalProvider: boolValue(values["experimental_provider"]),
	}

	// Environment variable overrides
	if val := os.Getenv("AC_FEATURE_VERBOSE_LOGGING"); val != "" {
		f.EnableVerboseLogging = isTrue(val)
	}
	if val := os.Getenv("AC_FEATURE_PARALLEL_EXECUTION"); val != "" {
		f.EnableParallelExecution = isTrue(val)
	}
	if val := os.Getenv("AC_FEATURE_EXPERIMENTAL_PROVIDER"); val != "" {
		f.UseExperimentalProvider = isTrue(val)
	}

	return f
}

func boolValue(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		return isTrue(s)
	}
	return false
}

func isTrue(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}
