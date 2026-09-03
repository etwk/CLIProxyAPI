package thinking_test

import (
	"errors"
	"testing"

	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/claude"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

func TestClaudeFable51RejectsDisabledThinking(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "disabled type", body: `{"thinking":{"type":"disabled"}}`},
		{name: "zero budget", body: `{"thinking":{"type":"enabled","budget_tokens":0}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := thinking.ApplyThinking([]byte(test.body), "claude-fable-5-1", "claude", "claude", "claude")
			if err == nil {
				t.Fatal("ApplyThinking() error = nil, want always-on thinking validation error")
			}
			var thinkingErr *thinking.ThinkingError
			if !errors.As(err, &thinkingErr) {
				t.Fatalf("ApplyThinking() error = %T %v, want *thinking.ThinkingError", err, err)
			}
			if thinkingErr.Code != thinking.ErrThinkingCannotBeDisabled {
				t.Fatalf("ApplyThinking() error code = %q, want %q", thinkingErr.Code, thinking.ErrThinkingCannotBeDisabled)
			}
		})
	}
}

func TestClaudeFable51ConvertsManualBudgetToAdaptiveEffort(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":8192}}`)
	out, err := thinking.ApplyThinking(body, "claude-fable-5-1", "claude", "claude", "claude")
	if err != nil {
		t.Fatalf("ApplyThinking() error = %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
		t.Fatalf("thinking.type = %q, want adaptive; body=%s", got, out)
	}
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("thinking.budget_tokens reached adaptive-only model: %s", out)
	}
	if got := gjson.GetBytes(out, "output_config.effort").String(); got != "medium" {
		t.Fatalf("output_config.effort = %q, want medium; body=%s", got, out)
	}
}
