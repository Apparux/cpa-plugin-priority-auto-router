package main

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestRoutePlanStoreConsumeDeletesOnePlan(t *testing.T) {
	store := newRoutePlanStore(time.Minute)
	key := routePlanKey{SourceFormat: "claude", Model: "code"}
	store.store(key, RoutePlan{ClientType: "claude", Candidates: []Candidate{{Name: "first"}}})

	plan, ok := store.consume(key)
	if !ok || plan.ClientType != "claude" || len(plan.Candidates) != 1 {
		t.Fatalf("consume = %#v, %v", plan, ok)
	}
	if _, ok := store.consume(key); ok {
		t.Fatalf("second consume found deleted plan")
	}
}

func TestRoutePlanStoreExpiredPlansIgnored(t *testing.T) {
	store := newRoutePlanStore(time.Nanosecond)
	key := routePlanKey{SourceFormat: "claude", Model: "code"}
	store.store(key, RoutePlan{ClientType: "claude", Candidates: []Candidate{{Name: "expired"}}})
	time.Sleep(time.Millisecond)
	if _, ok := store.consume(key); ok {
		t.Fatalf("consume found expired plan")
	}
}

func TestRoutePlanStoreConcurrentSafeQueue(t *testing.T) {
	store := newRoutePlanStore(time.Minute)
	key := routePlanKey{SourceFormat: "claude", Model: "code"}
	const count = 20
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.store(key, RoutePlan{ClientType: "claude", Candidates: []Candidate{{Name: "candidate"}}})
		}()
	}
	wg.Wait()
	seen := 0
	for {
		if _, ok := store.consume(key); !ok {
			break
		}
		seen++
	}
	if seen != count {
		t.Fatalf("consumed %d plans, want %d", seen, count)
	}
}

func TestRoutePlanForExecutorRecomputesWhenPlanAbsent(t *testing.T) {
	p := testPlugin(t, baseTestConfig)
	plan, ok := p.routePlanForExecutor(pluginapi.ExecutorRequest{
		Model:        "code",
		SourceFormat: "claude",
		Headers:      http.Header{},
	})
	if !ok || plan.ClientType != "claude" || len(plan.Candidates) != 2 {
		t.Fatalf("plan = %#v, ok = %v", plan, ok)
	}
}
