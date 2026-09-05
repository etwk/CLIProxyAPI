package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

// This identity was verified against Astra in upstream issue #5521. It must
// remain available when the catalog has no overrides or another credential
// registers the same model without them (the remaining issue in PR #5522).
func TestCodexHeadersSupportAstraWithoutModelOverrides(t *testing.T) {
	const wantUA = "codex-tui/0.153.3 (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; 0.153.3)"
	for _, provider := range []string{"codex", "openai-compatibility"} {
		t.Run(provider, func(t *testing.T) {
			reg := registry.GetGlobalRegistry()
			clientID := "astra-header-test-" + provider
			reg.RegisterClient(clientID, provider, []*registry.ModelInfo{{ID: "gpt-6-astra"}})
			t.Cleanup(func() { reg.UnregisterClient(clientID) })
			auth := &cliproxyauth.Auth{Provider: "codex", Metadata: map[string]any{"access_token": "test-token"}}
			cfg := &config.Config{}
			req := httptest.NewRequest(http.MethodPost, "https://example.com/responses", nil)
			applyCodexHeaders(req, auth, "test-token", true, cfg)
			wsHeaders := applyCodexWebsocketHeaders(context.Background(), nil, auth, "test-token", cfg)
			for transport, headers := range map[string]http.Header{"http": req.Header, "websocket": wsHeaders} {
				applyModelHeaderOverrides(headers, "gpt-6-astra")
				if got := headers.Get("User-Agent"); got != wantUA {
					t.Errorf("%s User-Agent = %q, want %q", transport, got, wantUA)
				}
				if got := headers.Get("Originator"); got != "codex-tui" {
					t.Errorf("%s Originator = %q, want codex-tui", transport, got)
				}
			}
		})
	}
}

func TestCodexExecutorAstraPreservesNativeRequest(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "non-streaming"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			captured := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, errRead := io.ReadAll(r.Body)
				if errRead != nil {
					t.Errorf("read request: %v", errRead)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				captured <- body
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_astra\",\"object\":\"response\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"output\":[]}}\n\n")
			}))
			defer server.Close()

			executor := NewCodexExecutor(&config.Config{})
			auth := &cliproxyauth.Auth{
				Provider:   "codex",
				Attributes: map[string]string{"base_url": server.URL},
				Metadata:   map[string]any{"access_token": "test-token"},
			}
			// Codex resolves its local Ultra mode to Astra's advertised
			// multi_agent_reasoning_effort (xhigh) before making API requests.
			req := cliproxyexecutor.Request{
				Model:   "gpt-6-astra",
				Payload: []byte(`{"model":"gpt-6-astra","reasoning":{"effort":"xhigh"},"input":[{"type":"message","role":"user","content":"hello"},{"type":"configuration_update","reasoning":{"effort":"max"}}],"tools":[{"type":"function","name":"lookup","description":"Look up a value","parameters":{"type":"object","properties":{}},"async":true}]}`),
			}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, Stream: stream}
			if stream {
				result, errStream := executor.ExecuteStream(context.Background(), auth, req, opts)
				if errStream != nil {
					t.Fatal(errStream)
				}
				for chunk := range result.Chunks {
					if chunk.Err != nil {
						t.Fatal(chunk.Err)
					}
				}
			} else if _, errExecute := executor.Execute(context.Background(), auth, req, opts); errExecute != nil {
				t.Fatal(errExecute)
			}
			body := <-captured
			for path, want := range map[string]string{
				"model":                    "gpt-6-astra",
				"reasoning.effort":         "xhigh",
				"input.1.type":             "configuration_update",
				"input.1.reasoning.effort": "max",
				"tools.0.async":            "true",
			} {
				if got := gjson.GetBytes(body, path).String(); got != want {
					t.Errorf("%s = %q, want %q", path, got, want)
				}
			}
		})
	}
}
