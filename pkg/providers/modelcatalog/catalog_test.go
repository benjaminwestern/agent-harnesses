package modelcatalog

import "testing"

func TestPiCatalogAdvertisesThinkingOptions(t *testing.T) {
	models := Pi()
	if len(models) == 0 {
		t.Fatal("Pi catalog is empty")
	}
	caps := models[0].Capabilities
	if !caps.SupportsThinkingLevel {
		t.Fatal("Pi catalog should advertise thinking-level support")
	}
	if len(caps.ReasoningEffortLevels) == 0 {
		t.Fatal("Pi catalog should expose reasoning effort options")
	}
}
