package thinking

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func TestUltraLevelParsingAndBudgetConversion(t *testing.T) {
	for _, suffix := range []string{"ultra", "Ultra", "ULTRA"} {
		level, ok := ParseLevelSuffix(suffix)
		if !ok || level != LevelUltra {
			t.Fatalf("ParseLevelSuffix(%q) = (%q, %v), want (%q, true)", suffix, level, ok, LevelUltra)
		}
	}

	budget, ok := ConvertLevelToBudget(string(LevelUltra))
	if !ok || budget != 128000 {
		t.Fatalf("ConvertLevelToBudget(%q) = (%d, %v), want (128000, true)", LevelUltra, budget, ok)
	}
}

func TestUltraLevelPreservesOrFallsBackToHighestSupportedIntent(t *testing.T) {
	tests := []struct {
		name      string
		supported []string
		want      ThinkingLevel
	}{
		{name: "preserve ultra", supported: []string{"high", "xhigh", "max", "ultra"}, want: LevelUltra},
		{name: "prefer max", supported: []string{"high", "xhigh", "max"}, want: LevelMax},
		{name: "then xhigh", supported: []string{"high", "xhigh"}, want: LevelXHigh},
		{name: "then high", supported: []string{"high"}, want: LevelHigh},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modelInfo := &registry.ModelInfo{Thinking: &registry.ThinkingSupport{Levels: tc.supported}}
			if got := mapConfiguredHighIntent(LevelUltra, modelInfo); got != tc.want {
				t.Fatalf("mapConfiguredHighIntent(ultra) = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUltraLevelConvertsForBudgetOnlyProvider(t *testing.T) {
	modelInfo := &registry.ModelInfo{
		ID:       "budget-only-model",
		Type:     "claude",
		Thinking: &registry.ThinkingSupport{Min: 1024, Max: 64000},
	}
	got, err := ValidateConfig(
		ThinkingConfig{Mode: ModeLevel, Level: LevelUltra},
		modelInfo,
		"openai-response",
		"claude",
		false,
	)
	if err != nil {
		t.Fatalf("ValidateConfig(ultra) error = %v", err)
	}
	if got.Mode != ModeBudget || got.Budget != 64000 || got.Level != "" {
		t.Fatalf("ValidateConfig(ultra) = %+v, want a clamped 64000-token budget", got)
	}
}
