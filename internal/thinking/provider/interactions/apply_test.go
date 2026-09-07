package interactions

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

func TestNormalizeInteractionsUltraLevel(t *testing.T) {
	if got := normalizeInteractionsLevel(string(thinking.LevelUltra), nil); got != string(thinking.LevelHigh) {
		t.Fatalf("normalizeInteractionsLevel(ultra, nil) = %q, want high", got)
	}

	astra := &registry.ModelInfo{Thinking: &registry.ThinkingSupport{Levels: []string{"high", "max", "ultra"}}}
	if got := normalizeInteractionsLevel(string(thinking.LevelUltra), astra); got != string(thinking.LevelUltra) {
		t.Fatalf("normalizeInteractionsLevel(ultra, Astra) = %q, want ultra", got)
	}
}
