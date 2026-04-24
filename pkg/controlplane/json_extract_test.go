package controlplane

import "testing"

func TestExtractStructuredJSONFindsTrailingObject(t *testing.T) {
	rendered, normalised := ExtractStructuredJSON("tool noise\n{\"ok\":true}", func(candidate string) (string, string, bool) {
		if candidate == "{\"ok\":true}" {
			return "ok", candidate, true
		}
		return "", "", false
	})
	if rendered != "ok" {
		t.Fatalf("rendered = %q, want ok", rendered)
	}
	if normalised != "{\"ok\":true}" {
		t.Fatalf("normalised = %q, want JSON object", normalised)
	}
}

func TestJSONObjectCandidatesFindsBalancedObject(t *testing.T) {
	candidates := JSONObjectCandidates("prefix {\"a\":1} suffix")
	if len(candidates) == 0 {
		t.Fatal("expected at least one JSON candidate")
	}
	if candidates[0] == "" {
		t.Fatal("expected non-empty JSON candidate")
	}
}
