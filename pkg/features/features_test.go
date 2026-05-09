package features

import "testing"

func TestCheck(t *testing.T) {
	// Test environment variable
	flag := Flag("TEST_FLAG")
	t.Setenv("AGENTIC_CONTROL_FEATURE_TEST_FLAG", "true")

	if !Check(flag) {
		t.Errorf("Expected TEST_FLAG to be enabled via env var")
	}

	// Test registry override
	Set(flag, false)
	if Check(flag) {
		t.Errorf("Expected TEST_FLAG to be disabled via registry override")
	}

	// Test Apply
	Apply(map[string]bool{"another_flag": true})
	if !Check(Flag("ANOTHER_FLAG")) {
		t.Errorf("Expected ANOTHER_FLAG to be enabled via Apply")
	}
}
