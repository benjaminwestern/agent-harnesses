package interaction

import "testing"

func TestAllMethodsHasCurrentAgenticInteractionSurface(t *testing.T) {
	const expectedCurrentMethodCount = 102
	if len(AllMethods) != expectedCurrentMethodCount {
		t.Fatalf("AllMethods has %d methods, want %d", len(AllMethods), expectedCurrentMethodCount)
	}

	seen := make(map[string]struct{}, len(AllMethods))
	for _, method := range AllMethods {
		if method == "" {
			t.Fatal("AllMethods contains an empty method")
		}
		if _, ok := seen[method]; ok {
			t.Fatalf("AllMethods contains duplicate method %q", method)
		}
		seen[method] = struct{}{}
	}
}
