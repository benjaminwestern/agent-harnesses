package features

import (
	"os"
	"testing"
)

func TestFromMap(t *testing.T) {
	m := map[string]any{
		"verbose_logging":    true,
		"parallel_execution": "true",
		"experimental_provider": 1, // Invalid, should be false
	}

	f := FromMap(m)

	if !f.EnableVerboseLogging {
		t.Errorf("expected EnableVerboseLogging to be true")
	}
	if !f.EnableParallelExecution {
		t.Errorf("expected EnableParallelExecution to be true")
	}
	if f.UseExperimentalProvider {
		t.Errorf("expected UseExperimentalProvider to be false")
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	os.Setenv("AC_FEATURE_VERBOSE_LOGGING", "true")
	defer os.Unsetenv("AC_FEATURE_VERBOSE_LOGGING")

	m := map[string]any{
		"verbose_logging": false,
	}

	f := FromMap(m)

	if !f.EnableVerboseLogging {
		t.Errorf("expected EnableVerboseLogging to be true (overridden by env)")
	}
}

func TestIsTrue(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"random", false},
	}

	for _, tt := range tests {
		if got := isTrue(tt.input); got != tt.expected {
			t.Errorf("isTrue(%q) = %v; want %v", tt.input, got, tt.expected)
		}
	}
}
