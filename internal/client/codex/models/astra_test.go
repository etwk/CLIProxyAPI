package models

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

func TestCodexClientModelsResponseAstraKeepsUltraModeAndWireEffort(t *testing.T) {
	response := BuildResponseForClient([]map[string]any{{"id": "gpt-6-astra"}}, nil, false, "0.153.3")
	models := response["models"].([]map[string]any)
	if len(models) != 1 {
		t.Fatalf("got %d models, want Astra", len(models))
	}
	astra := models[0]
	if got := astra["multi_agent_reasoning_effort"]; got != "xhigh" {
		t.Fatalf("multi_agent_reasoning_effort = %v, want xhigh", got)
	}
	levels := astra["supported_reasoning_levels"].([]any)
	var hasUltra bool
	for _, rawLevel := range levels {
		if rawLevel.(map[string]any)["effort"] == "ultra" {
			hasUltra = true
		}
	}
	if !hasUltra {
		t.Fatal("Astra client catalog lost Ultra orchestration mode")
	}
	model := registry.LookupModelInfo("gpt-6-astra", "codex")
	if model == nil || model.Thinking == nil {
		t.Fatal("missing Astra upstream thinking capabilities")
	}
	for _, effort := range model.Thinking.Levels {
		if effort == "ultra" {
			t.Fatal("client-only Ultra mode must not be advertised as a wire effort")
		}
	}
}
