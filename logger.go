package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func logCandidateSuccess(hostCallbackID string, plan RoutePlan, candidate Candidate, status int) {
	safeLog(hostCallbackID, "info", "priority auto router candidate succeeded", map[string]any{
		"client_type":        plan.ClientType,
		"requested_model":    plan.RequestedModel,
		"stream":             plan.Stream,
		"selected_candidate": candidate.Name,
		"candidate_priority": candidate.Priority,
		"status_code":        status,
		"final_result":       "success",
	})
}

func logFallback(hostCallbackID string, plan RoutePlan, failed Candidate, next Candidate, status int, err error) {
	safeLog(hostCallbackID, "warn", "priority auto router fallback", map[string]any{
		"client_type":        plan.ClientType,
		"requested_model":    plan.RequestedModel,
		"stream":             plan.Stream,
		"selected_candidate": failed.Name,
		"next_candidate":     next.Name,
		"candidate_priority": failed.Priority,
		"fallback_reason":    fallbackReason(status, err),
		"status_code":        status,
	})
}

func logNoFallback(hostCallbackID string, plan RoutePlan, candidate Candidate, status int, err error) {
	safeLog(hostCallbackID, "warn", "priority auto router no fallback", map[string]any{
		"client_type":        plan.ClientType,
		"requested_model":    plan.RequestedModel,
		"stream":             plan.Stream,
		"selected_candidate": candidate.Name,
		"candidate_priority": candidate.Priority,
		"fallback_reason":    fallbackReason(status, err),
		"status_code":        status,
		"final_result":       "failed",
	})
}

func fallbackReason(status int, err error) string {
	if status > 0 {
		return fmt.Sprintf("status_%d", status)
	}
	if err != nil {
		if isNetworkError(err) {
			return "network_error"
		}
		return "host_error"
	}
	return "unknown"
}

func safeLog(hostCallbackID, level, message string, fields map[string]any) {
	fields = safeLogFields(fields)
	if strings.TrimSpace(hostCallbackID) != "" {
		_, err := callHostCallback(pluginabi.MethodHostLog, hostLogRequest{
			HostCallbackID: strings.TrimSpace(hostCallbackID),
			Level:          strings.TrimSpace(level),
			Message:        message,
			Fields:         fields,
		})
		if err == nil {
			return
		}
	}
	log.Printf("[%s] %s %v", pluginName, message, fields)
}

func safeLogFields(input map[string]any) map[string]any {
	allowed := map[string]struct{}{
		"request_id":         {},
		"client_type":        {},
		"requested_model":    {},
		"stream":             {},
		"sorted_candidates":  {},
		"selected_candidate": {},
		"next_candidate":     {},
		"candidate_priority": {},
		"fallback_reason":    {},
		"status_code":        {},
		"final_result":       {},
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if _, ok := allowed[key]; ok {
			out[key] = value
		}
	}
	return out
}
