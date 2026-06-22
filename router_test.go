package main

import (
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestRouteModelDetectsClaudeBySourceFormat(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "anthropic", Headers: http.Header{}})
	if !resp.Handled || resp.TargetKind != pluginapi.ModelRouteTargetSelf || resp.Reason != "plugin_priority_auto_router_claude" {
		t.Fatalf("route response = %#v", resp)
	}
}

func TestRouteModelDetectsCodexByUserAgent(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", Headers: http.Header{"User-Agent": {"Codex CLI/1.0"}}})
	if !resp.Handled || resp.TargetKind != pluginapi.ModelRouteTargetSelf || resp.Reason != "plugin_priority_auto_router_codex" {
		t.Fatalf("route response = %#v", resp)
	}
}

func TestRouteModelNonCodeUnhandled(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "not-code", SourceFormat: "claude", Headers: http.Header{}})
	if resp.Handled {
		t.Fatalf("Handled = true, want false")
	}
}

func TestRouteModelBypassUnhandled(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude", Headers: http.Header{bypassHeaderName: {"1"}}})
	if resp.Handled || resp.Reason != "priority_auto_router_bypass" {
		t.Fatalf("route response = %#v", resp)
	}
}

func TestRouteModelUnknownClientUnhandled(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "gemini", Headers: http.Header{}})
	if resp.Handled {
		t.Fatalf("Handled = true, want false")
	}
}

func TestRouteModelFiltersAvailableProviders(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude", Headers: http.Header{}, AvailableProviders: []string{"codex"}})
	if !resp.Handled {
		t.Fatalf("Handled = false, want true")
	}
	plan, ok := p.plans.consume(routePlanKeyFromExecutorRequest(executorRequest("claude", false)))
	if !ok {
		t.Fatalf("stored route plan not found")
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Provider != "codex" {
		t.Fatalf("candidates = %#v, want only codex provider", plan.Candidates)
	}
}

func TestRouteModelNoAvailableCandidatesUnhandled(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude", Headers: http.Header{}, AvailableProviders: []string{"gemini"}})
	if resp.Handled {
		t.Fatalf("Handled = true, want false")
	}
}

func TestRouteModelDoesNotFilterWhenAvailableProvidersAbsent(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	resp := p.routeModel(pluginapi.ModelRouteRequest{RequestedModel: "code", SourceFormat: "claude", Headers: http.Header{}})
	if !resp.Handled {
		t.Fatalf("Handled = false, want true")
	}
	plan, ok := p.plans.consume(routePlanKeyFromExecutorRequest(executorRequest("claude", false)))
	if !ok {
		t.Fatalf("stored route plan not found")
	}
	if len(plan.Candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(plan.Candidates))
	}
}
