package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

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

func TestModelCatalogLoadsPreserveFableAndNativeAstraCapabilities(t *testing.T) {
	// Match the remote catalog before its missing capabilities are corrected.
	const raw = `{
		"claude":[{"id":"claude-fable-5-1","thinking":{"min":1024,"max":128000,"zero_allowed":true,"levels":["low","medium","high","xhigh","max"]}}],
		"codex-team":[{"id":"gpt-6-astra","thinking":{"levels":["low","medium","high","xhigh","max"]},"native_capabilities":{"web_search":true}}],
		"codex-plus":[{"id":"gpt-6-astra","thinking":{"levels":["low","medium","high","xhigh","max"]},"native_capabilities":{"web_search":true}}],
		"codex-pro":[{"id":"gpt-6-astra","thinking":{"levels":["low","medium","high","xhigh","max"]},"native_capabilities":{"web_search":true}}]
	}`
	wantLevels := []string{"low", "medium", "high", "xhigh", "max"}
	assertCapabilities := func(t *testing.T, catalog *staticModelsJSON) {
		t.Helper()
		fable := catalog.Claude[0].Thinking
		if fable == nil || !fable.AlwaysOn || !fable.DynamicAllowed || fable.ZeroAllowed || fable.Min != 0 || fable.Max != 0 {
			t.Fatalf("Fable lost always-on adaptive thinking: %+v", fable)
		}
		for _, models := range [][]*ModelInfo{catalog.CodexTeam, catalog.CodexPlus, catalog.CodexPro} {
			astra := models[0]
			if astra.Thinking == nil || !slices.Equal(astra.Thinking.Levels, wantLevels) {
				t.Fatalf("Astra native capabilities changed: %+v", astra.Thinking)
			}
			if astra.NativeCapabilities == nil || astra.NativeCapabilities.WebSearch == nil || !*astra.NativeCapabilities.WebSearch {
				t.Fatal("normalization removed native web-search metadata")
			}
		}
	}

	previous := getModels()
	previousURLs := modelsURLs
	t.Cleanup(func() {
		modelsURLs = previousURLs
		modelsCatalogStore.mu.Lock()
		modelsCatalogStore.data = previous
		modelsCatalogStore.mu.Unlock()
	})

	t.Run("local catalog load", func(t *testing.T) {
		if err := loadModelsFromBytes([]byte(raw), "test stale catalog"); err != nil {
			t.Fatal(err)
		}
		assertCapabilities(t, getModels())
	})
	t.Run("remote refresh fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(raw)); err != nil {
				t.Errorf("write catalog: %v", err)
			}
		}))
		defer server.Close()
		modelsURLs = []string{server.URL}
		for range 2 {
			catalog, source := fetchModelsFromRemote(context.Background())
			if catalog == nil || source != server.URL {
				t.Fatalf("fetch returned catalog=%v source=%q", catalog, source)
			}
			assertCapabilities(t, catalog)
		}
	})
}
