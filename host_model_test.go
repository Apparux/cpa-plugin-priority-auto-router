package main

import (
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestBuildHostModelExecutionRequestSanitizesAndForwards(t *testing.T) {
	req := pluginapi.ExecutorRequest{
		SourceFormat:    "anthropic",
		Format:          "claude",
		OriginalRequest: []byte(`{"model":"code","source":"original"}`),
		Payload:         []byte(`{"model":"code","source":"payload"}`),
		Headers: http.Header{
			"Authorization": {"Bearer secret"},
			"X-API-Key":     {"secret"},
			"User-Agent":    {"ClaudeCode"},
		},
		Alt: "alt-a",
	}
	candidate := Candidate{Name: "claude_api", Model: "claude-target"}
	got := buildHostModelExecutionRequest(req, candidate, false, "callback-123")

	if got.Model != "claude-target" || got.Stream {
		t.Fatalf("model/stream = %q/%v", got.Model, got.Stream)
	}
	if got.EntryProtocol != "claude" || got.ExitProtocol != "claude" {
		t.Fatalf("protocols = %q/%q", got.EntryProtocol, got.ExitProtocol)
	}
	if got.Headers.Get("Authorization") != "" || got.Headers.Get("X-API-Key") != "" {
		t.Fatalf("sensitive headers were forwarded: %#v", got.Headers)
	}
	if got.Headers.Get("User-Agent") != "ClaudeCode" || got.Headers.Get(bypassHeaderName) != "1" {
		t.Fatalf("safe/bypass headers = %#v", got.Headers)
	}
	if string(got.Body) != string(req.OriginalRequest) {
		t.Fatalf("body = %s, want original request", got.Body)
	}
	if got.Alt != "alt-a" || got.HostCallbackID != "callback-123" {
		t.Fatalf("alt/callback = %q/%q", got.Alt, got.HostCallbackID)
	}
}

func TestBuildHostModelExecutionRequestFallsBackToPayload(t *testing.T) {
	req := pluginapi.ExecutorRequest{Format: "openai", Payload: []byte(`{"model":"code"}`)}
	got := buildHostModelExecutionRequest(req, Candidate{Model: "target"}, true, "")
	if string(got.Body) != string(req.Payload) || !got.Stream {
		t.Fatalf("body/stream = %s/%v", got.Body, got.Stream)
	}
}
