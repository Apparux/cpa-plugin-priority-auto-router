package main

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func (p *priorityAutoRouterPlugin) RouteModel(_ context.Context, req pluginapi.ModelRouteRequest) (pluginapi.ModelRouteResponse, error) {
	return p.routeModel(req), nil
}

func (p *priorityAutoRouterPlugin) routeModel(req pluginapi.ModelRouteRequest) pluginapi.ModelRouteResponse {
	if p == nil {
		return pluginapi.ModelRouteResponse{Handled: false}
	}
	if hasBypassMarker(req.Headers) {
		return pluginapi.ModelRouteResponse{Handled: false, Reason: "priority_auto_router_bypass"}
	}
	plan, ok := p.buildRoutePlan(req, true)
	if !ok {
		return pluginapi.ModelRouteResponse{Handled: false}
	}
	if p.plans != nil {
		p.plans.store(routePlanKeyFromRouteRequest(req), plan)
	}
	return pluginapi.ModelRouteResponse{
		Handled:    true,
		TargetKind: pluginapi.ModelRouteTargetSelf,
		Reason:     "plugin_priority_auto_router_" + plan.ClientType,
	}
}

func (p *priorityAutoRouterPlugin) buildRoutePlan(req pluginapi.ModelRouteRequest, filterProviders bool) (RoutePlan, bool) {
	if p == nil {
		return RoutePlan{}, false
	}
	if !modelConfigured(p.cfg.ClientModels, req.RequestedModel) {
		return RoutePlan{}, false
	}
	clientType := detectClient(req, p.cfg)
	if clientType == "" {
		return RoutePlan{}, false
	}
	client, ok := p.cfg.Clients[clientType]
	if !ok {
		return RoutePlan{}, false
	}
	candidates := cloneCandidates(client.Candidates)
	if filterProviders {
		candidates = filterByAvailableProviders(candidates, req.AvailableProviders)
	}
	candidates = sortCandidates(candidates)
	if len(candidates) == 0 {
		return RoutePlan{}, false
	}
	return RoutePlan{
		ClientType:     clientType,
		RequestedModel: strings.TrimSpace(req.RequestedModel),
		Stream:         req.Stream,
		Candidates:     candidates,
	}, true
}

func (p *priorityAutoRouterPlugin) routePlanForExecutor(req pluginapi.ExecutorRequest) (RoutePlan, bool) {
	if p == nil {
		return RoutePlan{}, false
	}
	if p.plans != nil {
		if plan, ok := p.plans.consume(routePlanKeyFromExecutorRequest(req)); ok {
			return plan, true
		}
	}
	body := req.OriginalRequest
	if len(body) == 0 {
		body = req.Payload
	}
	return p.buildRoutePlan(pluginapi.ModelRouteRequest{
		SourceFormat:   firstNonEmpty(req.SourceFormat, req.Format),
		RequestedModel: req.Model,
		Stream:         req.Stream,
		Headers:        cloneHeader(req.Headers),
		Query:          cloneValues(req.Query),
		Body:           append([]byte(nil), body...),
		Metadata:       cloneAnyMap(req.Metadata),
	}, false)
}

func hasBypassMarker(headers http.Header) bool {
	if strings.TrimSpace(headers.Get(bypassHeaderName)) == "1" {
		return true
	}
	for key, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), bypassHeaderName) {
			continue
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "1" {
				return true
			}
		}
	}
	return false
}

func modelConfigured(models []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model), requested) {
			return true
		}
	}
	return false
}

func detectClient(req pluginapi.ModelRouteRequest, cfg PluginConfig) string {
	source := normalizeProtocol(req.SourceFormat)
	for _, name := range sortedClientNames(cfg.Clients) {
		client := cfg.Clients[name]
		if containsNormalizedProtocol(client.SourceFormats, source) {
			return name
		}
	}
	ua := normalizedUserAgent(req.Headers)
	for _, name := range sortedClientNames(cfg.Clients) {
		client := cfg.Clients[name]
		if containsAny(ua, client.UserAgentContains) {
			return name
		}
	}
	return ""
}

func sortedClientNames(clients map[string]ClientConfig) []string {
	names := make([]string, 0, len(clients))
	for name := range clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func containsNormalizedProtocol(formats []string, source string) bool {
	for _, format := range formats {
		if normalizeProtocol(format) == source {
			return true
		}
	}
	return false
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func filterByAvailableProviders(candidates []Candidate, available []string) []Candidate {
	if len(available) == 0 {
		return candidates
	}
	providers := make(map[string]struct{}, len(available))
	for _, provider := range available {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider != "" {
			providers[provider] = struct{}{}
		}
	}
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := providers[strings.ToLower(strings.TrimSpace(candidate.Provider))]; ok {
			out = append(out, candidate)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneHeader(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	cloned := make(http.Header, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func cloneValues(values url.Values) url.Values {
	if values == nil {
		return nil
	}
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
