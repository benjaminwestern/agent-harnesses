package features

import (
	"os"
	"strings"
)

// Flag represents a feature flag.
type Flag string

const (
	// Example flag
	ExperimentalUI Flag = "EXPERIMENTAL_UI"
)

// IsEnabled checks if a feature flag is enabled.
// It checks environment variables first (prefixed with AGENTIC_CONTROL_FEATURE_).
func IsEnabled(flag Flag) bool {
	envVar := "AGENTIC_CONTROL_FEATURE_" + string(flag)
	val := strings.ToLower(os.Getenv(envVar))
	return val == "true" || val == "1" || val == "yes"
}

// Registry stores the state of feature flags if they are set via config.
var Registry = make(map[Flag]bool)

// Set sets the state of a feature flag.
func Set(flag Flag, enabled bool) {
	Registry[flag] = enabled
}

// Apply updates the registry with the provided flags.
func Apply(flags map[string]bool) {
	for name, enabled := range flags {
		Registry[Flag(strings.ToUpper(name))] = enabled
	}
}

// Check checks if a feature flag is enabled, considering both Registry and environment variables.
func Check(flag Flag) bool {
	if enabled, ok := Registry[flag]; ok {
		return enabled
	}
	return IsEnabled(flag)
}
