package registry

import "testing"

func TestNormalizeClaudeFable51Capabilities(t *testing.T) {
	otherThinking := &ThinkingSupport{Min: 1, Max: 2, ZeroAllowed: true}
	catalog := &staticModelsJSON{Claude: []*ModelInfo{
		{
			ID: "claude-fable-5-1",
			Thinking: &ThinkingSupport{
				Min:         1024,
				Max:         128000,
				ZeroAllowed: true,
				Levels:      []string{"low"},
			},
		},
		{ID: "other-claude-model", Thinking: otherThinking},
	}}

	normalizeClaudeFable51Capabilities(catalog)

	fableThinking := catalog.Claude[0].Thinking
	if fableThinking == nil || !fableThinking.AlwaysOn || !fableThinking.DynamicAllowed {
		t.Fatalf("Claude Fable 5.1 thinking = %+v, want always-on adaptive thinking", fableThinking)
	}
	if fableThinking.Min != 0 || fableThinking.Max != 0 || fableThinking.ZeroAllowed {
		t.Fatalf("Claude Fable 5.1 thinking = %+v, want no manual budget or disable support", fableThinking)
	}
	wantLevels := []string{"low", "medium", "high", "xhigh", "max"}
	if len(fableThinking.Levels) != len(wantLevels) {
		t.Fatalf("Claude Fable 5.1 levels = %v, want %v", fableThinking.Levels, wantLevels)
	}
	for i, want := range wantLevels {
		if fableThinking.Levels[i] != want {
			t.Fatalf("Claude Fable 5.1 levels = %v, want %v", fableThinking.Levels, wantLevels)
		}
	}
	if catalog.Claude[1].Thinking != otherThinking {
		t.Fatal("unrelated model capabilities were replaced")
	}
}
