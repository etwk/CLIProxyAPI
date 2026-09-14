package auth

import (
	"context"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

func TestWeightedSelectionSkipsZeroWeightBeforePriority(t *testing.T) {
	for _, affinity := range []bool{false, true} {
		name := "weighted"
		var selector Selector = &WeightedRoundRobinSelector{}
		if affinity {
			name = "session-affinity"
			sessionSelector := NewSessionAffinitySelector(selector)
			t.Cleanup(sessionSelector.Stop)
			selector = sessionSelector
		}
		t.Run(name, func(t *testing.T) {
			manager := NewManager(nil, selector, nil)
			candidates := []*Auth{
				{ID: "high-zero", Provider: "codex", Status: StatusActive, Attributes: map[string]string{"priority": "10", AttributeWeight: "0"}},
				{ID: "low-positive", Provider: "codex", Status: StatusActive, Attributes: map[string]string{"priority": "1", AttributeWeight: "1"}},
			}
			priority, available, err := manager.availableAuthsForSelector(selector, candidates, "codex", "gpt-6-astra", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(priority) != 1 || priority[0].ID != "low-positive" {
				t.Fatalf("zero-weight credential hid a usable priority tier: %+v", priority)
			}
			ctx := selectorContextForAvailableAuths(context.Background(), selector, "gpt-6-astra")
			selected, err := selector.Pick(ctx, "codex", selectionArgForSelector(selector, "gpt-6-astra"), cliproxyexecutor.Options{}, available)
			if err != nil || selected == nil || selected.ID != "low-positive" {
				t.Fatalf("selection = %+v, error = %v; want low-positive", selected, err)
			}
		})
	}
}
