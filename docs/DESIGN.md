# DESIGN.md - CPA Plugin Priority Auto Router 插件设计

## 1. 技术方案

插件采用：

```text
model_router + executor + host.model.*
```

原因：

```text
1. model_router 可以在 CPA 常规 provider/auth 解析前拦截 model=code。
2. 单独 model_router 只能决定一次路由，不能处理“失败后 fallback”。
3. 需要 fallback 时，model_router 应返回 TargetKind=self。
4. executor 中按 priority 排序后的 candidates 依次调用 host.model.execute / host.model.execute_stream。
5. host.model.* 复用 CPA 宿主已有模型执行链路、凭证、代理、日志、用量统计。
6. 插件不直接读取、不保存、不打印任何 API Key 或 OAuth token。
```

整体流程：

```text
客户端请求 model=code
  -> CPA 调用 priority-auto-router.model.route
  -> 插件识别 client_type
  -> 插件读取 candidates
  -> 插件按 priority 排序
  -> 插件返回 TargetKind=self
  -> CPA 调用插件 executor
  -> executor 按排序后的 candidates 调 host.model.*
  -> 成功则返回
  -> 失败且允许 fallback，则调用下一个 candidate
  -> 全部失败则返回最后错误或统一错误
```

## 2. 插件能力声明

插件注册时声明：

```json
{
  "schema_version": 1,
  "metadata": {
    "Name": "priority-auto-router",
    "Version": "0.1.0",
    "Author": "Amin",
    "Description": "Priority-based auto router for Claude Code, Codex CLI, API-key providers and Codex OAuth fallback."
  },
  "capabilities": {
    "model_router": true,
    "executor": true,
    "executor_model_scope": "static",
    "executor_input_formats": ["claude", "anthropic", "openai", "responses", "chat-completions"],
    "executor_output_formats": ["claude", "anthropic", "openai", "responses", "chat-completions"]
  }
}
```

注意：

```text
executor_input_formats / executor_output_formats 需要以当前 CPA SDK 真实枚举为准。
如果 SDK 中 Claude 协议枚举是 anthropic，就用 anthropic。
如果 SourceFormat 是 claude，model.route 仍需支持 claude。
```



## 2.2 buildPlugin 设计

`main.go` 中建议提供：

```go
var pluginVersion = "0.1.0"

func buildPlugin(configYAML []byte, pluginDir string) (pluginapi.Plugin, error) {
    cfg, err := parsePriorityAutoRouterConfig(configYAML)
    if err != nil {
        return pluginapi.Plugin{}, err
    }

    p := &priorityAutoRouterPlugin{
        cfg: cfg,
        configYAML: append([]byte(nil), configYAML...),
        pluginDir: pluginDir,
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
                    Description: "Client matchers and route candidates.",
                },
                {
                    Name:        "fallback",
                    Type:        pluginapi.ConfigFieldTypeObject,
                    Description: "Fallback status codes and streaming fallback policy.",
                },
            },
        },
        Capabilities: pluginapi.Capabilities{
            ModelRouter: p,
            Executor:    p,
        },
    }, nil
}
```

具体字段名以当前 `sdk/pluginapi` 为准。如果 SDK 中能力字段名不是 `ModelRouter` / `Executor`，必须读取源码后调整，不能臆测。

## 2.1 插件工程形态

本插件采用独立仓库形态，参考 `cpa-plugin-jshandler`：

```text
cpa-plugin-priority-auto-router/
  Makefile
  README.md
  abi.go
  main.go
  config.go
  router.go
  executor.go
  fallback.go
  host_model.go
  logger.go
  route_plan.go
  shared_library_unix.go
  shared_library_windows.go
```

关键命名：

```go
const pluginName = "priority-auto-router"
const pluginProvider = "priority-auto-router"

var pluginVersion = "0.1.0"
```

导出函数建议命名：

```go
//export cliproxy_plugin_init
func cliproxy_plugin_init(...)

//export PriorityAutoRouterPluginCall
func PriorityAutoRouterPluginCall(...)

//export PriorityAutoRouterPluginFree
func PriorityAutoRouterPluginFree(...)

//export PriorityAutoRouterPluginShutdown
func PriorityAutoRouterPluginShutdown()
```

注意：

```text
Go exported C 函数不能使用 hyphen，所以函数名使用 PriorityAutoRouterPluginCall。
动态库文件名仍然使用 priority-auto-router.so / .dylib / .dll。
```

## 3. 配置结构

### 3.1 PluginConfig

```go
type PluginConfig struct {
    ClientModels []string                `yaml:"client_models"`
    Clients      map[string]ClientConfig `yaml:"clients"`
    Fallback     FallbackConfig          `yaml:"fallback"`
}
```

### 3.2 ClientConfig

```go
type ClientConfig struct {
    SourceFormats     []string    `yaml:"source_formats"`
    UserAgentContains []string    `yaml:"user_agent_contains"`
    Candidates        []Candidate `yaml:"candidates"`
}
```

### 3.3 Candidate

```go
type Candidate struct {
    Name        string `yaml:"name"`
    ChannelType string `yaml:"channel_type"`
    Provider    string `yaml:"provider"`
    Model       string `yaml:"model"`
    Priority    int    `yaml:"priority"`
    Order       int    `yaml:"-"`
}
```

字段说明：

```text
Name:
  候选通道名称，例如 claude_api、codex_api_key、codex_oauth。

ChannelType:
  通道类型。
  支持：
    claude_api_key
    codex_api_key
    codex_oauth

Provider:
  CPA 内置 provider key，例如 claude、codex。

Model:
  传给 host.model.execute / host.model.execute_stream 的实际模型名。
  例如：
    claude/code
    thirdcodex/code
    oauth-codex

Priority:
  数字越大越优先。

Order:
  配置原始顺序，仅用于排序兜底。
```

### 3.4 FallbackConfig

```go
type FallbackConfig struct {
    Enabled                            bool  `yaml:"enabled"`
    FallbackOnStatus                   []int `yaml:"fallback_on_status"`
    NoFallbackOnStatus                 []int `yaml:"no_fallback_on_status"`
    StreamFallbackBeforeFirstChunkOnly bool  `yaml:"stream_fallback_before_first_chunk_only"`
}
```

## 4. 默认配置

如果用户没有设置部分字段，使用默认值：

```yaml
client_models:
  - "code"

fallback:
  enabled: true
  fallback_on_status: [401, 403, 408, 409, 429, 500, 502, 503, 504]
  no_fallback_on_status: [400, 404, 422]
  stream_fallback_before_first_chunk_only: true
```

## 5. 客户端识别

识别优先级：

```text
1. SourceFormat
2. User-Agent
3. 其他 headers
4. default_client，如果配置存在
5. 无法识别则 Handled=false
```

伪代码：

```go
func detectClient(req ModelRouteRequest, cfg PluginConfig) string {
    source := strings.ToLower(req.SourceFormat)
    ua := strings.ToLower(getHeader(req.Headers, "user-agent"))

    for name, client := range cfg.Clients {
        if matchAny(source, client.SourceFormats) {
            return name
        }
    }

    for name, client := range cfg.Clients {
        if containsAny(ua, client.UserAgentContains) {
            return name
        }
    }

    return ""
}
```

## 6. Candidate 排序

排序规则：

```text
1. priority 大的排前面。
2. priority 相同，channel_type = codex_oauth 的排前面。
3. priority 和 channel_type 都相同，按 YAML 配置原始顺序。
```

伪代码：

```go
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
            if a.ChannelType == "codex_oauth" {
                return true
            }

            if b.ChannelType == "codex_oauth" {
                return false
            }
        }

        return a.Order < b.Order
    })

    return candidates
}
```

## 7. model.route 逻辑

### 7.1 输入字段

需要读取：

```text
PluginID
SourceFormat
RequestedModel
Stream
Headers
Query
Body
Metadata
AvailableProviders
host_callback_id
```

### 7.2 路由流程

伪代码：

```go
func ModelRoute(req ModelRouteRequest) ModelRouteResponse {
    cfg := loadConfig()

    if hasBypassMarker(req.Headers) {
        return ModelRouteResponse{Handled: false}
    }

    if !contains(cfg.ClientModels, req.RequestedModel) {
        return ModelRouteResponse{Handled: false}
    }

    clientType := detectClient(req, cfg)
    if clientType == "" {
        return ModelRouteResponse{Handled: false}
    }

    candidates := cfg.Clients[clientType].Candidates
    candidates = filterByAvailableProviders(candidates, req.AvailableProviders)
    candidates = sortCandidates(candidates)

    if len(candidates) == 0 {
        return ModelRouteResponse{Handled: false}
    }

    saveRoutePlan(req, RoutePlan{
        RequestID:      requestID(req),
        ClientType:     clientType,
        RequestedModel: req.RequestedModel,
        Stream:         req.Stream,
        Candidates:     candidates,
    })

    return ModelRouteResponse{
        Handled:    true,
        TargetKind: "self",
        Reason:    "plugin_priority_auto_router_" + clientType,
    }
}
```

## 8. RoutePlan

```go
type RoutePlan struct {
    RequestID      string
    ClientType     string
    RequestedModel string
    Stream         bool
    Candidates     []Candidate
}
```

RoutePlan 存储策略：

```text
优先使用 request metadata / internal context。
如果 SDK 无法直接从 model.route 传 plan 到 executor，则用 request_id + TTL map 临时保存。
TTL 建议 5 分钟。
executor 执行完成后清理。
```

## 9. executor.execute 非流式逻辑

伪代码：

```go
func Execute(req ExecutorRequest) ExecutorResponse {
    plan := loadRoutePlan(req)
    if len(plan.Candidates) == 0 {
        return errorResponse("no route candidates")
    }

    var lastErr error
    var lastStatus int
    var lastCandidate Candidate

    for i, candidate := range plan.Candidates {
        resp, err := callHostModelExecute(req, candidate)

        if err == nil && isSuccess(resp) {
            logSuccess(plan, candidate, i)
            return resp
        }

        status := extractStatus(resp, err)
        lastErr = err
        lastStatus = status
        lastCandidate = candidate

        if !shouldFallback(status, err) {
            logNoFallback(plan, candidate, status, err)
            return buildErrorResponse(resp, err)
        }

        logFallback(plan, candidate, status, err)
    }

    return buildAllFailedError(lastCandidate, lastStatus, lastErr)
}
```

## 10. 调用 host.model.execute

请求核心字段：

```json
{
  "entry_protocol": "<req.SourceFormat 或 req.Format>",
  "exit_protocol": "<req.SourceFormat 或 req.Format>",
  "model": "<candidate.Model>",
  "stream": false,
  "body": "<original request body base64>",
  "headers": {
    "X-CPA-Priority-Auto-Router-Bypass": "1"
  },
  "query": {},
  "alt": ""
}
```

注意：

```text
1. model 使用 candidate.Model。
2. body 使用原始客户端请求体。
3. 不要复制客户端 Authorization 到 host.model.execute。
4. 调用 host.model.* 时转发 host_callback_id，防止递归调用自身插件。
5. 如果 SDK 已经通过 host_callback_id 跳过自身插件，则 bypass header 作为双保险。
```

## 11. executor.execute_stream 流式逻辑

伪代码：

```go
func ExecuteStream(req ExecutorRequest) ExecutorStreamResponse {
    plan := loadRoutePlan(req)

    for i, candidate := range plan.Candidates {
        stream, err := callHostModelExecuteStream(req, candidate)

        if err != nil {
            status := extractStatus(nil, err)
            if shouldFallback(status, err) {
                logFallback(plan, candidate, status, err)
                continue
            }

            return streamError(err)
        }

        firstChunk, err := hostModelStreamRead(stream.ID)
        if err != nil {
            hostModelStreamClose(stream.ID)

            status := extractStatus(nil, err)
            if shouldFallback(status, err) {
                logFallback(plan, candidate, status, err)
                continue
            }

            return streamError(err)
        }

        emitFirstChunk(firstChunk)

        for {
            chunk, err := hostModelStreamRead(stream.ID)
            if err == io.EOF {
                hostModelStreamClose(stream.ID)
                closeClientStream()
                return success
            }

            if err != nil {
                hostModelStreamClose(stream.ID)
                return streamError(err)
            }

            emitChunk(chunk)
        }
    }

    return finalStreamError("all candidates failed")
}
```

## 12. 流式 fallback 限制

```text
只允许在第一个有效 chunk 发送给客户端前 fallback。
一旦 emitFirstChunk 成功，就不允许 fallback。
```

## 13. fallback 判断

伪代码：

```go
func shouldFallback(status int, err error, cfg FallbackConfig) bool {
    if !cfg.Enabled {
        return false
    }

    if containsInt(cfg.NoFallbackOnStatus, status) {
        return false
    }

    if containsInt(cfg.FallbackOnStatus, status) {
        return true
    }

    if isNetworkError(err) {
        return true
    }

    return false
}
```

## 14. AvailableProviders 过滤

伪代码：

```go
func filterByAvailableProviders(candidates []Candidate, available []string) []Candidate {
    result := make([]Candidate, 0, len(candidates))

    for _, c := range candidates {
        if contains(available, c.Provider) {
            result = append(result, c)
        }
    }

    return result
}
```

说明：

```text
provider = claude 时，需要 AvailableProviders 包含 claude。
provider = codex 时，需要 AvailableProviders 包含 codex。
```

## 15. 防递归设计

优先使用 CPA 提供的 `host_callback_id`：

```text
executor 调 host.model.* 时转发 host_callback_id。
宿主据此跳过同一个插件自身的请求、响应和流式拦截器。
```

同时增加 bypass header：

```text
X-CPA-Priority-Auto-Router-Bypass: 1
```

`model.route` 看到该 header 直接返回：

```json
{
  "Handled": false
}
```

## 16. 配置校验

必须校验：

```text
1. client_models 非空。
2. clients 非空。
3. 每个 client 至少一个 candidate。
4. candidate.name 非空。
5. candidate.channel_type 非空。
6. candidate.provider 非空。
7. candidate.model 非空。
8. candidate.priority 必须是整数。
9. channel_type 只允许：
   - claude_api_key
   - codex_api_key
   - codex_oauth
10. 同一个 client 下 candidate.name 不允许重复。
11. fallback 状态码必须在 100-599。
```

## 17. 日志

路由开始日志：

```text
[priority-auto-router] request_id=xxx client=claude model=code stream=false candidates=[claude_api:100,codex_oauth:90]
```

同 priority OAuth 优先日志：

```text
[priority-auto-router] request_id=xxx client=codex candidates=[codex_oauth:100,codex_api_key:100] tie_break=codex_oauth_first
```

fallback 日志：

```text
[priority-auto-router] request_id=xxx failed=codex_api_key status=429 next=codex_oauth
```

最终成功日志：

```text
[priority-auto-router] request_id=xxx final=codex_oauth result=success
```

最终失败日志：

```text
[priority-auto-router] request_id=xxx result=failed candidates=2 last_status=503
```

## 18. 测试重点

单元测试：

```text
1. RequestedModel != code 返回 Handled=false。
2. SourceFormat=claude 命中 claude client。
3. User-Agent 包含 codex 命中 codex client。
4. priority 高的排前面。
5. priority 相同 codex_oauth 排前面。
6. priority 相同且都不是 codex_oauth，保持配置顺序。
7. AvailableProviders 过滤不可用 provider。
8. 429 触发 fallback。
9. 400 不触发 fallback。
10. 网络错误触发 fallback。
11. 流式首 chunk 前失败触发 fallback。
12. 流式首 chunk 后失败不触发 fallback。
13. bypass header 生效。
14. host_callback_id 正确透传。
```
