package main

import (
	"strings"
	"testing"
)

func TestParseConfigAppliesDefaults(t *testing.T) {
	cfg, err := parsePriorityAutoRouterConfig([]byte(baseTestConfig))
	if err != nil {
		t.Fatalf("parsePriorityAutoRouterConfig() error = %v", err)
	}
	if len(cfg.ClientModels) != 1 || cfg.ClientModels[0] != "code" {
		t.Fatalf("ClientModels = %#v, want [code]", cfg.ClientModels)
	}
	if !cfg.Fallback.Enabled || !cfg.Fallback.StreamFallbackBeforeFirstChunkOnly {
		t.Fatalf("Fallback defaults = %#v", cfg.Fallback)
	}
	if !statusInList(429, cfg.Fallback.FallbackOnStatus) || !statusInList(400, cfg.Fallback.NoFallbackOnStatus) {
		t.Fatalf("Fallback status defaults = %#v", cfg.Fallback)
	}
}

func TestParseConfigRejectsInvalidChannel(t *testing.T) {
	_, err := parsePriorityAutoRouterConfig([]byte(`
clients:
  claude:
    candidates:
      - name: bad
        channel_type: direct_http
        provider: claude
        model: claude/code
        priority: 1
`))
	if err == nil || !strings.Contains(err.Error(), "unsupported channel_type") {
		t.Fatalf("error = %v, want unsupported channel_type", err)
	}
}

func TestParseConfigRejectsDuplicateCandidateName(t *testing.T) {
	_, err := parsePriorityAutoRouterConfig([]byte(`
clients:
  claude:
    candidates:
      - name: same
        channel_type: claude_api_key
        provider: claude
        model: claude/code
        priority: 1
      - name: same
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
        priority: 1
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate candidate") {
		t.Fatalf("error = %v, want duplicate candidate", err)
	}
}

func TestParseConfigRejectsInvalidStatus(t *testing.T) {
	_, err := parsePriorityAutoRouterConfig([]byte(baseTestConfig + `
fallback:
  fallback_on_status: [99]
`))
	if err == nil || !strings.Contains(err.Error(), "invalid HTTP status") {
		t.Fatalf("error = %v, want invalid HTTP status", err)
	}
}
