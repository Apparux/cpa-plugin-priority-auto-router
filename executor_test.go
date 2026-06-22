package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestExecuteFirstCandidateSuccess(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	calls := 0
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		calls++
		if method != pluginabi.MethodHostModelExecute {
			t.Fatalf("method = %q", method)
		}
		req := payload.(hostModelExecutionRequest)
		if req.Model != "claude/code" || req.Headers.Get(bypassHeaderName) != "1" {
			t.Fatalf("host request = %#v", req)
		}
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusOK, Body: []byte(`ok`), Headers: http.Header{"X-Test": {"yes"}}}), nil
	})

	resp, err := p.execute(nil, executorRequest("claude", false), "callback-1")
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if calls != 1 || string(resp.Payload) != "ok" || resp.Headers.Get("X-Test") != "yes" || resp.Metadata["selected_candidate"] != "claude_api" {
		t.Fatalf("calls/resp = %d/%#v", calls, resp)
	}
}

func TestExecuteFallbackOn429ThenSuccess(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	models := []string{}
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		req := payload.(hostModelExecutionRequest)
		models = append(models, req.Model)
		if len(models) == 1 {
			return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusTooManyRequests, Body: []byte(`rate limited`)}), nil
		}
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusOK, Body: []byte(`second ok`)}), nil
	})

	resp, err := p.execute(nil, executorRequest("claude", false), "callback-2")
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if fmt.Sprint(models) != "[claude/code oauth-codex]" || string(resp.Payload) != "second ok" {
		t.Fatalf("models/resp = %v/%s", models, resp.Payload)
	}
}

func TestExecuteNoFallbackOn400(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	calls := 0
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		calls++
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusBadRequest, Body: []byte(`bad request`)}), nil
	})

	_, err := p.execute(nil, executorRequest("claude", false), "callback-3")
	if err == nil {
		t.Fatalf("execute() error = nil, want error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestExecuteAllFallbackableFailuresReturnsSafeError(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	calls := 0
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		calls++
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusTooManyRequests}), nil
	})

	_, err := p.execute(nil, executorRequest("claude", false), "callback-4")
	if err == nil || err.Error() != "model execution failed with status 429" {
		t.Fatalf("error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestExecuteForwardsHostCallbackID(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	withHostCallback(t, func(method string, payload any) (json.RawMessage, error) {
		req := payload.(hostModelExecutionRequest)
		if req.HostCallbackID != "executor-callback" {
			t.Fatalf("HostCallbackID = %q", req.HostCallbackID)
		}
		return rawJSON(t, pluginapi.HostModelExecutionResponse{StatusCode: http.StatusOK}), nil
	})
	if _, err := p.execute(nil, executorRequest("claude", false), "executor-callback"); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
}
