package main

import "github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

const (
	pluginName     = "priority-auto-router"
	pluginProvider = "priority-auto-router"
)

var pluginVersion = "0.1.1"

var executorFormats = []string{
	"claude",
	"anthropic",
	"openai",
	"responses",
	"chat-completions",
	"codex",
}

type priorityAutoRouterPlugin struct {
	cfg        PluginConfig
	configYAML []byte
	pluginDir  string
	plans      *routePlanStore
}

func main() {}

func buildPlugin(configYAML []byte, pluginDir string) (pluginapi.Plugin, error) {
	cfg, err := parsePriorityAutoRouterConfig(configYAML)
	if err != nil {
		return pluginapi.Plugin{}, err
	}

	p := &priorityAutoRouterPlugin{
		cfg:        cfg,
		configYAML: append([]byte(nil), configYAML...),
		pluginDir:  pluginDir,
		plans:      newRoutePlanStore(defaultRoutePlanTTL),
	}

	return pluginapi.Plugin{
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "Amin",
			GitHubRepository: "https://github.com/router-for-me/cpa-plugin-priority-auto-router",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "client_models",
					Type:        pluginapi.ConfigFieldTypeArray,
					Description: "Client-facing model names handled by this plugin, such as code.",
				},
				{
					Name:        "clients",
					Type:        pluginapi.ConfigFieldTypeObject,
					Description: "Client matchers and route candidates for Claude Code and Codex CLI.",
				},
				{
					Name:        "fallback",
					Type:        pluginapi.ConfigFieldTypeObject,
					Description: "Fallback status codes and streaming fallback policy.",
				},
			},
		},
		Capabilities: pluginapi.Capabilities{
			ModelRouter:           p,
			Executor:              p,
			ExecutorModelScope:    pluginapi.ExecutorModelScopeStatic,
			ExecutorInputFormats:  append([]string(nil), executorFormats...),
			ExecutorOutputFormats: append([]string(nil), executorFormats...),
		},
	}, nil
}
