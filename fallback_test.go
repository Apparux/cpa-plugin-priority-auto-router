package main

import (
	"context"
	"errors"
	"testing"
)

func TestShouldFallbackOnConfiguredStatuses(t *testing.T) {
	cfg := defaultPluginConfig().Fallback
	for _, status := range []int{429, 503} {
		if !shouldFallback(status, nil, cfg) {
			t.Fatalf("shouldFallback(%d) = false, want true", status)
		}
	}
}

func TestShouldNotFallbackOnConfiguredTerminalStatuses(t *testing.T) {
	cfg := defaultPluginConfig().Fallback
	for _, status := range []int{400, 404, 422} {
		if shouldFallback(status, nil, cfg) {
			t.Fatalf("shouldFallback(%d) = true, want false", status)
		}
	}
}

func TestShouldFallbackOnNetworkError(t *testing.T) {
	cfg := defaultPluginConfig().Fallback
	if !shouldFallback(0, context.DeadlineExceeded, cfg) {
		t.Fatalf("deadline exceeded should be fallbackable")
	}
	if !shouldFallback(0, errors.New("connection reset by peer"), cfg) {
		t.Fatalf("connection reset should be fallbackable")
	}
}

func TestShouldNotFallbackWhenDisabled(t *testing.T) {
	cfg := defaultPluginConfig().Fallback
	cfg.Enabled = false
	if shouldFallback(429, nil, cfg) {
		t.Fatalf("disabled fallback returned true")
	}
}

func TestStatusFromErrorParsesStatusCode(t *testing.T) {
	if got := statusFromError(errors.New("upstream failed with status 429")); got != 429 {
		t.Fatalf("statusFromError() = %d, want 429", got)
	}
}
