package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestABIMethodRegisterAndIdentifier(t *testing.T) {
	registerTestPlugin(t)
	raw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodExecutorIdentifier, nil)
	if err != nil {
		t.Fatalf("identifier method error = %v", err)
	}
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("envelope = %#v", env)
	}
	var got abiIdentifierResponse
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if got.Identifier != pluginProvider {
		t.Fatalf("Identifier = %q", got.Identifier)
	}
}

func TestABIRegisterAllowsInstallOnlyConfigAndLeavesRoutingUnhandled(t *testing.T) {
	raw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodPluginRegister, mustMarshal(t, abiLifecycleRequest{ConfigYAML: []byte("enabled: true\npriority: 0\n")}))
	if err != nil {
		t.Fatalf("register method error = %v", err)
	}
	t.Cleanup(func() {
		priorityAutoRouterABIState.Lock()
		priorityAutoRouterABIState.plugin = nil
		priorityAutoRouterABIState.shuttingDown = false
		priorityAutoRouterABIState.Unlock()
	})
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("register envelope = %#v", env)
	}
	var got abiRegistration
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if got.SchemaVersion != pluginabi.SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, pluginabi.SchemaVersion)
	}
	if got.Metadata.Name != pluginName || got.Metadata.GitHubRepository != "https://github.com/Apparux/cpa-plugin-priority-auto-router" {
		t.Fatalf("Metadata = %#v", got.Metadata)
	}
	if !got.Capabilities.ModelRouter || !got.Capabilities.Executor {
		t.Fatalf("Capabilities = %#v", got.Capabilities)
	}
	if got.Capabilities.ExecutorModelScope != pluginapi.ExecutorModelScopeStatic {
		t.Fatalf("ExecutorModelScope = %q", got.Capabilities.ExecutorModelScope)
	}
	if len(got.Capabilities.ExecutorInputFormats) == 0 || len(got.Capabilities.ExecutorOutputFormats) == 0 {
		t.Fatalf("executor formats missing: %#v", got.Capabilities)
	}

	routeRaw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodModelRoute, mustMarshal(t, rpcModelRouteRequest{ModelRouteRequest: pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude"}}))
	if err != nil {
		t.Fatalf("model.route method error = %v", err)
	}
	var routeEnv abiEnvelope
	if err := json.Unmarshal(routeRaw, &routeEnv); err != nil {
		t.Fatalf("json.Unmarshal(route) error = %v", err)
	}
	if !routeEnv.OK {
		t.Fatalf("route envelope = %#v", routeEnv)
	}
	var route pluginapi.ModelRouteResponse
	if err := json.Unmarshal(routeEnv.Result, &route); err != nil {
		t.Fatalf("json.Unmarshal(route result) error = %v", err)
	}
	if route.Handled {
		t.Fatalf("Handled = true, want false")
	}
}

func TestABIRegisterRejectsMalformedExplicitRouteConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "unsupported channel",
			config: `clients:
  claude:
    candidates:
      - name: bad
        channel_type: direct_http
        provider: claude
        model: claude/code
`,
			wantErr: "unsupported channel_type",
		},
		{
			name: "duplicate candidate",
			config: `clients:
  claude:
    candidates:
      - name: same
        channel_type: claude_api_key
        provider: claude
        model: claude/code
      - name: same
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
`,
			wantErr: "duplicate candidate",
		},
		{
			name: "invalid fallback status",
			config: `clients:
  claude:
    candidates:
      - name: claude_api
        channel_type: claude_api_key
        provider: claude
        model: claude/code
fallback:
  fallback_on_status: [99]
`,
			wantErr: "invalid HTTP status",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodPluginRegister, mustMarshal(t, abiLifecycleRequest{ConfigYAML: []byte(tt.config)}))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestABIRouteWrapperHandlesHostCallbackIDField(t *testing.T) {
	registerTestPlugin(t)
	req := rpcModelRouteRequest{
		ModelRouteRequest: pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude"},
		HostCallbackID:    "router-callback",
	}
	raw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodModelRoute, mustMarshal(t, req))
	if err != nil {
		t.Fatalf("model.route method error = %v", err)
	}
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("envelope = %#v", env)
	}
	var got pluginapi.ModelRouteResponse
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("json.Unmarshal(result) error = %v", err)
	}
	if !got.Handled || got.TargetKind != pluginapi.ModelRouteTargetSelf {
		t.Fatalf("route response = %#v", got)
	}
}

func TestABIExecuteWrapperForwardsExecutorHostCallbackID(t *testing.T) {
	registerTestPlugin(t)
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		req := payload.(hostModelExecutionRequest)
		if req.HostCallbackID != "executor-callback" {
			t.Fatalf("HostCallbackID = %q", req.HostCallbackID)
		}
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: 200}), nil
	})
	req := rpcExecutorRequest{ExecutorRequest: executorRequest("claude", false), HostCallbackID: "executor-callback"}
	raw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodExecutorExecute, mustMarshal(t, req))
	if err != nil {
		t.Fatalf("executor.execute method error = %v", err)
	}
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("envelope = %#v", env)
	}
}

func registerTestPlugin(t *testing.T) {
	t.Helper()
	raw, err := handlePriorityAutoRouterABIMethod(context.Background(), pluginabi.MethodPluginRegister, mustMarshal(t, abiLifecycleRequest{ConfigYAML: []byte(baseTestConfig)}))
	if err != nil {
		t.Fatalf("register method error = %v", err)
	}
	var env abiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !env.OK {
		t.Fatalf("register envelope = %#v", env)
	}
	t.Cleanup(func() {
		priorityAutoRouterABIState.Lock()
		priorityAutoRouterABIState.plugin = nil
		priorityAutoRouterABIState.shuttingDown = false
		priorityAutoRouterABIState.Unlock()
	})
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}
