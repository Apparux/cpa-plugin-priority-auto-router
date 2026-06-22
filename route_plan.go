package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const defaultRoutePlanTTL = 5 * time.Minute
const idempotencyMetadataKey = "idempotency_key"

type RoutePlan struct {
	ClientType     string
	RequestedModel string
	Stream         bool
	Candidates     []Candidate
	CreatedAt      time.Time
}

type routePlanKey struct {
	SourceFormat string
	Model        string
	Stream       bool
	UserAgent    string
	Idempotency  string
}

type storedRoutePlan struct {
	plan      RoutePlan
	expiresAt time.Time
}

type routePlanStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	plans map[string][]storedRoutePlan
}

func newRoutePlanStore(ttl time.Duration) *routePlanStore {
	if ttl <= 0 {
		ttl = defaultRoutePlanTTL
	}
	return &routePlanStore{ttl: ttl, plans: make(map[string][]storedRoutePlan)}
}

func (s *routePlanStore) store(key routePlanKey, plan RoutePlan) {
	if s == nil {
		return
	}
	now := time.Now()
	plan.CreatedAt = now
	plan.Candidates = cloneCandidates(plan.Candidates)
	encoded := key.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	s.plans[encoded] = append(s.plans[encoded], storedRoutePlan{plan: plan, expiresAt: now.Add(s.ttl)})
}

func (s *routePlanStore) consume(key routePlanKey) (RoutePlan, bool) {
	if s == nil {
		return RoutePlan{}, false
	}
	now := time.Now()
	encoded := key.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	queue := s.plans[encoded]
	if len(queue) == 0 {
		return RoutePlan{}, false
	}
	entry := queue[0]
	queue = queue[1:]
	if len(queue) == 0 {
		delete(s.plans, encoded)
	} else {
		s.plans[encoded] = queue
	}
	entry.plan.Candidates = cloneCandidates(entry.plan.Candidates)
	return entry.plan, true
}

func (s *routePlanStore) clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans = make(map[string][]storedRoutePlan)
}

func (s *routePlanStore) cleanupLocked(now time.Time) {
	for key, queue := range s.plans {
		kept := queue[:0]
		for _, entry := range queue {
			if now.Before(entry.expiresAt) {
				kept = append(kept, entry)
			}
		}
		if len(kept) == 0 {
			delete(s.plans, key)
			continue
		}
		s.plans[key] = kept
	}
}

func (k routePlanKey) String() string {
	return fmt.Sprintf("%s\x00%s\x00%t\x00%s\x00%s", k.SourceFormat, k.Model, k.Stream, k.UserAgent, k.Idempotency)
}

func routePlanKeyFromRouteRequest(req pluginapi.ModelRouteRequest) routePlanKey {
	return routePlanKey{
		SourceFormat: normalizeProtocol(req.SourceFormat),
		Model:        strings.ToLower(strings.TrimSpace(req.RequestedModel)),
		Stream:       req.Stream,
		UserAgent:    normalizedUserAgent(req.Headers),
		Idempotency:  metadataString(req.Metadata, idempotencyMetadataKey),
	}
}

func routePlanKeyFromExecutorRequest(req pluginapi.ExecutorRequest) routePlanKey {
	return routePlanKey{
		SourceFormat: normalizeProtocol(firstNonEmpty(req.SourceFormat, req.Format)),
		Model:        strings.ToLower(strings.TrimSpace(req.Model)),
		Stream:       req.Stream,
		UserAgent:    normalizedUserAgent(req.Headers),
		Idempotency:  metadataString(req.Metadata, idempotencyMetadataKey),
	}
}

func normalizedUserAgent(headers http.Header) string {
	return strings.ToLower(strings.TrimSpace(headers.Get("User-Agent")))
}

func metadataString(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	if raw, ok := meta[key]; ok {
		if value, okString := raw.(string); okString {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneCandidates(input []Candidate) []Candidate {
	if input == nil {
		return nil
	}
	out := make([]Candidate, len(input))
	copy(out, input)
	return out
}
