package helps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestClaudeVersionFloorPreservesNativeDetectionSignals(t *testing.T) {
	for _, version := range []string{"2.1.258", "2.1.272", "2.2.0", "3.0.0"} {
		headers := confirmedClaudeCodeHeaders()
		headers.Set("User-Agent", "claude-cli/"+version+" (external, cli)")
		if got := DetectClaudeCodeRequest(headers, claudeCodeDetectionPayload(validClaudeCodeMetadataUserID), false); !got.Confirmed {
			t.Errorf("genuine client %s was not confirmed: %+v", version, got)
		}
		for _, missing := range []string{"X-App", "Anthropic-Beta", "User-Agent"} {
			incomplete := headers.Clone()
			incomplete.Del(missing)
			if got := DetectClaudeCodeRequest(incomplete, claudeCodeDetectionPayload(validClaudeCodeMetadataUserID), false); got.Confirmed {
				t.Errorf("client %s was confirmed without %s", version, missing)
			}
		}
		if got := DetectClaudeCodeRequest(headers, []byte(`{}`), false); got.Confirmed {
			t.Errorf("client %s was confirmed without metadata", version)
		}
	}
	for _, version := range []string{"2.1.257", "4.0.0", "999.0.0", "2.1.272-bad"} {
		headers := confirmedClaudeCodeHeaders()
		headers.Set("User-Agent", "claude-cli/"+version+" (external, cli)")
		if got := DetectClaudeCodeRequest(headers, claudeCodeDetectionPayload(validClaudeCodeMetadataUserID), false); got.Confirmed {
			t.Errorf("client %s bypassed version validation", version)
		}
	}
}

func TestClaudeVersionFloorPreservesNewSoftwareHeaders(t *testing.T) {
	headers := claudeDeviceHeaders("claude-cli/2.1.272 (external, cli)")
	headers.Set("X-Stainless-Package-Version", "0.113.0")
	headers.Set("X-Stainless-Runtime-Version", "v26.4.0")
	for _, confirmed := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
		ApplyClaudeLegacyDeviceHeaders(req, headers, nil, confirmed)
		want := defaultClaudeDeviceProfile(nil)
		if confirmed {
			want.UserAgent, want.PackageVersion, want.RuntimeVersion = headers.Get("User-Agent"), "0.113.0", "v26.4.0"
		}
		if req.Header.Get("User-Agent") != want.UserAgent || req.Header.Get("X-Stainless-Package-Version") != want.PackageVersion || req.Header.Get("X-Stainless-Runtime-Version") != want.RuntimeVersion {
			t.Errorf("confirmed=%v headers=%v, want software tuple %+v", confirmed, req.Header, want)
		}
	}
}

func TestClaudeVersionFloorUpdatesLocalAndHomeProfiles(t *testing.T) {
	for _, homeMode := range []bool{false, true} {
		name := "local"
		if homeMode {
			name = "home"
		}
		t.Run(name, func(t *testing.T) {
			ResetClaudeDeviceProfileCache()
			t.Cleanup(ResetClaudeDeviceProfileCache)
			client := newFakeClaudeDeviceProfileKVClient()
			useFakeClaudeDeviceProfileKVClient(t, client, homeMode, nil)
			auth := &cliproxyauth.Auth{ID: "version-floor-" + name}
			cfg := &config.Config{}
			for _, tc := range []struct{ cli, sdk, runtime, wantCLI, wantSDK, wantRuntime string }{
				{"2.1.272", "0.112.1", "v26.3.0", "2.1.272", "0.112.1", "v26.3.0"},
				{"2.1.272", "0.113.0", "v26.3.0", "2.1.272", "0.113.0", "v26.3.0"},
				{"2.1.272", "0.113.0", "v26.4.0", "2.1.272", "0.113.0", "v26.4.0"},
				{"2.1.271", "0.112.1", "v26.3.0", "2.1.272", "0.113.0", "v26.4.0"},
				{"2.1.272", "0.112.1", "v26.5.0", "2.1.272", "0.113.0", "v26.4.0"},
			} {
				// Model expiration of the external lock deterministically between requests.
				for key := range client.values {
					if strings.HasPrefix(key, "cpa:claude:device-profile-lock:") {
						delete(client.values, key)
					}
				}
				headers := claudeDeviceHeaders("claude-cli/" + tc.cli + " (external, cli)")
				headers.Set("X-Stainless-Package-Version", tc.sdk)
				headers.Set("X-Stainless-Runtime-Version", tc.runtime)
				got, err := ResolveClaudeDeviceProfileRequired(context.Background(), auth, "test-key", headers, cfg)
				if err != nil {
					t.Fatal(err)
				}
				if got.UserAgent != "claude-cli/"+tc.wantCLI+" (external, cli)" || got.PackageVersion != tc.wantSDK || got.RuntimeVersion != tc.wantRuntime {
					t.Fatalf("profile=%+v, want %s/%s/%s", got, tc.wantCLI, tc.wantSDK, tc.wantRuntime)
				}
			}
		})
	}
}
