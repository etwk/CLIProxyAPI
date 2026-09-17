package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestClaudeToolChangesShareDeclarationAliases(t *testing.T) {
	body := []byte(`{"tools":[{"name":"SendFeedback","input_schema":{"type":"object","properties":{"name":{"type":"string"}}}},{"type":"web_search_20250305","name":"web_search"},{"name":"mcp__native__lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"start"},{"role":"system","content":[{"type":"tool_removal","tool":{"type":"tool_reference","name":"SendFeedback"}},{"type":"tool_addition","tool":{"type":"tool_reference","name":"SendFeedback"}},{"type":"tool_addition","tool":{"type":"tool_reference","name":"web_search"}},{"type":"tool_removal","tool":{"type":"tool_reference","name":"mcp__native__lookup"}},{"type":"tool_addition","tool":{"type":"tool_reference","name":"undeclared"}},{"type":"tool_addition","tool":{"type":"mcp_tool_reference","server_name":"remote","name":"SendFeedback"}},{"type":"tool_removal","tool":{"type":"mcp_toolset_reference","server_name":"remote"}},{"type":"text","text":"SendFeedback must not be rewritten in prose"}]},{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"SendFeedback","input":{"name":"SendFeedback"}}]}]}`)
	for name, remap := range map[string]func([]byte, claudeMCPAliasOptions) ([]byte, map[string]string){
		"batched": func(body []byte, opts claudeMCPAliasOptions) ([]byte, map[string]string) {
			out, reverse, ok := remapOAuthToolNamesWithBatchedEdits(body, opts)
			if !ok {
				t.Fatal("valid tool changes fell back to the legacy remapper")
			}
			return out, reverse
		},
		"legacy": remapOAuthToolNamesWithOptionsLegacy,
	} {
		t.Run(name, func(t *testing.T) {
			original := bytes.Clone(body)
			out, reverse := remap(body, claudeMCPAliasOptions{secret: "tool-changes-test"})
			alias := gjson.GetBytes(out, "tools.0.name").String()
			if alias == "SendFeedback" || alias == "" || reverse[alias] != "SendFeedback" {
				t.Fatalf("invalid declaration alias %q, reverse=%v", alias, reverse)
			}
			for _, path := range []string{"messages.1.content.0.tool.name", "messages.1.content.1.tool.name", "messages.2.content.0.name"} {
				if got := gjson.GetBytes(out, path).String(); got != alias {
					t.Errorf("%s = %q, want declared alias %q", path, got, alias)
				}
			}
			for _, path := range []string{"tools.0.input_schema", "tools.1", "tools.2", "messages.1.content.2", "messages.1.content.3", "messages.1.content.4", "messages.1.content.5", "messages.1.content.6", "messages.1.content.7", "messages.2.content.0.input"} {
				if gjson.GetBytes(out, path).Raw != gjson.GetBytes(body, path).Raw {
					t.Errorf("unrelated or externally scoped content changed at %s", path)
				}
			}
			if !bytes.Equal(body, original) {
				t.Fatal("remapper mutated caller-owned request bytes")
			}
		})
	}
}

func TestClaudeExecutorToolChangesAcrossRequestPaths(t *testing.T) {
	for _, native := range []bool{false, true} {
		client := "third-party"
		if native {
			client = "updated-native-client"
		}
		for _, path := range []string{"messages", "stream", "count_tokens"} {
			t.Run(client+"/"+path, func(t *testing.T) {
				type observedRequest struct {
					body    []byte
					headers http.Header
				}
				seen := make(chan observedRequest, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read upstream request: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					seen <- observedRequest{body, r.Header.Clone()}
					switch path {
					case "count_tokens":
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"input_tokens":12}`)
					case "stream":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-opus-5\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					default:
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-opus-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					}
				}))
				defer server.Close()
				headers := claudeNativeHelperHeaders("claude-code-20250219,mid-conversation-system-2026-04-07,mid-conversation-tool-changes-2026-07-01", "gzip, deflate, br, zstd")
				headers.Set("User-Agent", "claude-cli/2.1.272 (external, cli)")
				headers.Set("X-Stainless-Package-Version", "0.113.0")
				headers.Set("X-Stainless-Runtime-Version", "v26.4.0")
				if !native {
					headers.Set("User-Agent", "third-party/1.0")
				}
				body := []byte(`{"model":"claude-opus-5","max_tokens":128,"tools":[{"name":"SendFeedback","input_schema":{"type":"object","properties":{}}}],"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hello"},{"role":"system","content":[{"type":"tool_removal","tool":{"type":"tool_reference","name":"SendFeedback"}},{"type":"tool_addition","tool":{"type":"tool_reference","name":"SendFeedback"}}]},{"role":"user","content":"continue"}]}`)
				body, err := sjson.SetBytes(body, "metadata.user_id", claudeNativeHelperUserID)
				if err != nil {
					t.Fatal(err)
				}
				executor := NewClaudeExecutor(&config.Config{})
				auth := claudeNativeHelperOAuthAuth(server.URL)
				req := cliproxyexecutor.Request{Model: "claude-opus-5", Payload: body}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, Headers: headers}
				switch path {
				case "count_tokens":
					_, err = executor.countTokensUpstream(context.Background(), auth, req, opts)
				case "stream":
					var result *cliproxyexecutor.StreamResult
					result, err = executor.ExecuteStream(context.Background(), auth, req, opts)
					if err == nil {
						for chunk := range result.Chunks {
							if chunk.Err != nil {
								t.Fatal(chunk.Err)
							}
						}
					}
				default:
					_, err = executor.Execute(context.Background(), auth, req, opts)
				}
				if err != nil {
					t.Fatal(err)
				}
				captured := <-seen
				toolName := gjson.GetBytes(captured.body, "tools.0.name").String()
				if (toolName == "SendFeedback") != native {
					t.Fatalf("native=%v tool name=%q", native, toolName)
				}
				changes := 0
				for _, message := range gjson.GetBytes(captured.body, "messages").Array() {
					for _, block := range message.Get("content").Array() {
						if block.Get("type").String() != "tool_addition" && block.Get("type").String() != "tool_removal" {
							continue
						}
						changes++
						if message.Get("role").String() != "system" || block.Get("tool.name").String() != toolName {
							t.Fatalf("tool change no longer references a declared tool in a system message: %s", block.Raw)
						}
					}
				}
				if changes != 2 {
					t.Fatalf("tool changes = %d, want both preserved; body=%s", changes, captured.body)
				}
				if native {
					for _, header := range []string{"User-Agent", "X-Stainless-Package-Version", "X-Stainless-Runtime-Version"} {
						if captured.headers.Get(header) != headers.Get(header) {
							t.Errorf("%s = %q, want %q", header, captured.headers.Get(header), headers.Get(header))
						}
					}
				}
				if !json.Valid(captured.body) {
					t.Fatal("upstream request is invalid JSON")
				}
			})
		}
	}
}
