package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const baseTestConfig = `
clients:
  claude:
    source_formats: [claude, anthropic]
    user_agent_contains: [claude]
    candidates:
      - name: claude_api
        channel_type: claude_api_key
        provider: claude
        model: claude/code
        priority: 100
      - name: codex_oauth
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
        priority: 90
  codex:
    source_formats: [openai, responses, openai-response, codex]
    user_agent_contains: [codex]
    candidates:
      - name: codex_api_key
        channel_type: codex_api_key
        provider: codex
        model: thirdcodex/code
        priority: 100
      - name: codex_oauth
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
        priority: 90
`

func testPlugin(t *testing.T, raw string) *priorityAutoRouterPlugin {
	t.Helper()
	plugin, err := buildPlugin([]byte(raw), "")
	if err != nil {
		t.Fatalf("buildPlugin() error = %v", err)
	}
	p, ok := plugin.Capabilities.ModelRouter.(*priorityAutoRouterPlugin)
	if !ok || p == nil {
		t.Fatalf("plugin instance has type %T", plugin.Capabilities.ModelRouter)
	}
	return p
}

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func withHostCallback(t *testing.T, fn func(string, any) (json.RawMessage, error)) {
	t.Helper()
	old := callHostCallback
	callHostCallback = func(method string, payload any) (json.RawMessage, error) {
		if method == pluginabi.MethodHostLog {
			return rawJSON(t, map[string]any{}), nil
		}
		return fn(method, payload)
	}
	t.Cleanup(func() { callHostCallback = old })
}

func executorRequest(format string, stream bool) pluginapi.ExecutorRequest {
	return pluginapi.ExecutorRequest{
		Model:           "code",
		SourceFormat:    format,
		Format:          format,
		Stream:          stream,
		OriginalRequest: []byte(`{"model":"code"}`),
	}
}
