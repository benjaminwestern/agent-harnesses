package toolrepair

import "testing"

func TestExtractToolCallsRepairsTrailingCommas(t *testing.T) {
	calls := ExtractToolCalls(`{"name":"write","arguments":{"paths":["a.go","b.go",],"replaceAll":false,},}`)
	if len(calls) != 1 {
		t.Fatalf("ExtractToolCalls returned %d calls, want 1", len(calls))
	}
	if calls[0].Name != "write" {
		t.Fatalf("call name = %q, want write", calls[0].Name)
	}
	paths, ok := calls[0].Arguments["paths"].([]any)
	if !ok || len(paths) != 2 || paths[0] != "a.go" || paths[1] != "b.go" {
		t.Fatalf("paths = %#v, want [a.go b.go]", calls[0].Arguments["paths"])
	}
	if calls[0].Arguments["replaceAll"] != false {
		t.Fatalf("replaceAll = %#v, want false", calls[0].Arguments["replaceAll"])
	}
}
