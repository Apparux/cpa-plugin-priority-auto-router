package main

import (
	"context"
	"encoding/json"
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
