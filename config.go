package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	bypassHeaderName = "X-CPA-Priority-Auto-Router-Bypass"

	channelTypeClaudeAPIKey = "claude_api_key"
	channelTypeCodexAPIKey  = "codex_api_key"
	channelTypeCodexOAuth   = "codex_oauth"
)

var defaultFallbackOnStatus = []int{401, 403, 408, 409, 429, 500, 502, 503, 504}
var defaultNoFallbackOnStatus = []int{400, 404, 422}

type PluginConfig struct {
	ClientModels []string                `yaml:"client_models"`
	Clients      map[string]ClientConfig `yaml:"clients"`
	Fallback     FallbackConfig          `yaml:"fallback"`
}

type ClientConfig struct {
	SourceFormats     []string    `yaml:"source_formats"`
	UserAgentContains []string    `yaml:"user_agent_contains"`
	Candidates        []Candidate `yaml:"candidates"`
}

type Candidate struct {
	Name        string `yaml:"name"`
	ChannelType string `yaml:"channel_type"`
	Provider    string `yaml:"provider"`
	Model       string `yaml:"model"`
	Priority    int    `yaml:"priority"`
	Order       int    `yaml:"-"`
}

type FallbackConfig struct {
	Enabled                            bool  `yaml:"enabled"`
	FallbackOnStatus                   []int `yaml:"fallback_on_status"`
	NoFallbackOnStatus                 []int `yaml:"no_fallback_on_status"`
	StreamFallbackBeforeFirstChunkOnly bool  `yaml:"stream_fallback_before_first_chunk_only"`
}

func defaultPluginConfig() PluginConfig {
	return PluginConfig{
		ClientModels: []string{"code"},
		Fallback: FallbackConfig{
			Enabled:                            true,
			FallbackOnStatus:                   append([]int(nil), defaultFallbackOnStatus...),
			NoFallbackOnStatus:                 append([]int(nil), defaultNoFallbackOnStatus...),
			StreamFallbackBeforeFirstChunkOnly: true,
		},
	}
}

func parsePriorityAutoRouterConfig(raw []byte) (PluginConfig, error) {
	cfg := defaultPluginConfig()
	if strings.TrimSpace(string(raw)) != "" {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return PluginConfig{}, fmt.Errorf("invalid %s config: %w", pluginName, err)
		}
	}
	normalizeConfig(&cfg)
	if err := validateConfig(cfg); err != nil {
		return PluginConfig{}, err
	}
	return cfg, nil
}

func normalizeConfig(cfg *PluginConfig) {
	cfg.ClientModels = trimStringList(cfg.ClientModels, false)
	if cfg.Clients == nil {
		return
	}
	normalizedClients := make(map[string]ClientConfig, len(cfg.Clients))
	for name, client := range cfg.Clients {
		clientName := strings.ToLower(strings.TrimSpace(name))
		if clientName == "" {
			clientName = strings.TrimSpace(name)
		}
		client.SourceFormats = trimStringList(client.SourceFormats, true)
		for i := range client.SourceFormats {
			client.SourceFormats[i] = normalizeProtocol(client.SourceFormats[i])
		}
		client.UserAgentContains = trimStringList(client.UserAgentContains, true)
		for i := range client.Candidates {
			client.Candidates[i].Name = strings.TrimSpace(client.Candidates[i].Name)
			client.Candidates[i].ChannelType = strings.ToLower(strings.TrimSpace(client.Candidates[i].ChannelType))
			client.Candidates[i].Provider = strings.ToLower(strings.TrimSpace(client.Candidates[i].Provider))
			client.Candidates[i].Model = strings.TrimSpace(client.Candidates[i].Model)
			client.Candidates[i].Order = i
		}
		normalizedClients[clientName] = client
	}
	cfg.Clients = normalizedClients
}

func validateConfig(cfg PluginConfig) error {
	if len(cfg.ClientModels) == 0 {
		return fmt.Errorf("%s config requires at least one client_models entry", pluginName)
	}
	if len(cfg.Clients) == 0 {
		return fmt.Errorf("%s config requires at least one clients entry", pluginName)
	}
	for name, client := range cfg.Clients {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s config has an empty client name", pluginName)
		}
		if len(client.Candidates) == 0 {
			return fmt.Errorf("%s client %q requires at least one candidate", pluginName, name)
		}
		seen := make(map[string]struct{}, len(client.Candidates))
		for i, candidate := range client.Candidates {
			prefix := fmt.Sprintf("%s client %q candidate[%d]", pluginName, name, i)
			if candidate.Name == "" {
				return fmt.Errorf("%s requires name", prefix)
			}
			if _, exists := seen[candidate.Name]; exists {
				return fmt.Errorf("%s client %q has duplicate candidate name %q", pluginName, name, candidate.Name)
			}
			seen[candidate.Name] = struct{}{}
			if candidate.ChannelType == "" {
				return fmt.Errorf("%s requires channel_type", prefix)
			}
			if !supportedChannelType(candidate.ChannelType) {
				return fmt.Errorf("%s has unsupported channel_type %q", prefix, candidate.ChannelType)
			}
			if candidate.Provider == "" {
				return fmt.Errorf("%s requires provider", prefix)
			}
			if candidate.Model == "" {
				return fmt.Errorf("%s requires model", prefix)
			}
		}
	}
	if err := validateStatusCodes("fallback_on_status", cfg.Fallback.FallbackOnStatus); err != nil {
		return err
	}
	if err := validateStatusCodes("no_fallback_on_status", cfg.Fallback.NoFallbackOnStatus); err != nil {
		return err
	}
	return nil
}

func validateStatusCodes(field string, codes []int) error {
	for _, code := range codes {
		if code < 100 || code > 599 {
			return fmt.Errorf("%s config %s contains invalid HTTP status %d", pluginName, field, code)
		}
	}
	return nil
}

func supportedChannelType(channelType string) bool {
	switch channelType {
	case channelTypeClaudeAPIKey, channelTypeCodexAPIKey, channelTypeCodexOAuth:
		return true
	default:
		return false
	}
}

func trimStringList(input []string, lower bool) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if lower {
			item = strings.ToLower(item)
		}
		out = append(out, item)
	}
	return out
}

func sortCandidates(input []Candidate) []Candidate {
	candidates := make([]Candidate, len(input))
	copy(candidates, input)
	for i := range candidates {
		candidates[i].Order = i
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a := candidates[i]
		b := candidates[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.ChannelType != b.ChannelType {
			if a.ChannelType == channelTypeCodexOAuth {
				return true
			}
			if b.ChannelType == channelTypeCodexOAuth {
				return false
			}
		}
		return a.Order < b.Order
	})
	return candidates
}

func normalizeProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "anthropic":
		return "claude"
	case "responses", "openai-responses", "openai_responses":
		return "openai-response"
	case "chat-completions", "chat_completions", "openai-chat-completions", "openai_chat_completions":
		return "openai"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
