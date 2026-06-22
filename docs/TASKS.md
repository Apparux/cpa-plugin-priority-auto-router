# TASKS.md + 实现提示词 - CPA Plugin Priority Auto Router

## 1. 开发任务拆分

### Task 1：阅读 CPA 插件 SDK 与示例

先阅读：

```text
sdk/pluginapi/types.go
sdk/pluginabi/types.go
internal/pluginhost/model_router.go
internal/pluginhost/host_callbacks.go
examples/plugin/simple/go/main.go
examples/plugin/claude-web-search-router/go/main.go
examples/plugin/claude-web-search-router/go/fallback.go
examples/plugin/host-model-callback/go/main.go
```

确认：

```text
1. ModelRouteRequest / ModelRouteResponse 真实字段。
2. ExecutorRequest / ExecutorResponse 真实字段。
3. host.model.execute 请求结构。
4. host.model.execute_stream / stream_read / stream_close 使用方式。
5. host_callback_id 字段如何透传。
6. 当前 SDK 协议枚举：claude / anthropic / openai / responses / chat-completions。
```

### Task 2：创建独立插件仓库

仓库名：

```text
cpa-plugin-priority-auto-router
```

Go module：

```text
github.com/router-for-me/cpa-plugin-priority-auto-router
```

运行时插件 ID、配置 key、构建产物名：

```text
priority-auto-router
```

推荐文件结构：

```text
.github/
  workflows/
    build.yml
  scripts/
    package-release.go
.gitignore
LICENSE
Makefile
README.md
abi.go
abi_test.go
config.go
config_test.go
router.go
router_test.go
sort_test.go
executor.go
executor_test.go
fallback.go
fallback_test.go
host_model.go
logger.go
route_plan.go
route_plan_test.go
main.go
shared_library_unix.go
shared_library_windows.go
go.mod
go.sum
config.example.yaml
```


### Task 2.1 实现 Makefile

参考作者插件的 Makefile，提供：

```makefile
PLUGIN_NAME ?= priority-auto-router
VERSION ?= 0.1.0
BUILD_DIR ?= .
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GO_LDFLAGS ?= -s -w -X main.pluginVersion=$(VERSION)

EXT_linux = so
EXT_freebsd = so
EXT_darwin = dylib
EXT_windows = dll

PLUGIN_EXT = $(or $(EXT_$(GOOS)),so)
PLUGIN_OUTPUT ?= $(BUILD_DIR)/$(PLUGIN_NAME).$(PLUGIN_EXT)
PLUGIN_HEADER = $(basename $(PLUGIN_OUTPUT)).h

.PHONY: build clean

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags "$(GO_LDFLAGS)" -o $(PLUGIN_OUTPUT) .
	rm -f $(PLUGIN_HEADER)

clean:
	rm -f $(BUILD_DIR)/$(PLUGIN_NAME).so
	rm -f $(BUILD_DIR)/$(PLUGIN_NAME).dylib
	rm -f $(BUILD_DIR)/$(PLUGIN_NAME).dll
	rm -f $(PLUGIN_HEADER)
```

### Task 2.2 实现 GitHub Actions 发布工作流

新增：

```text
.github/workflows/build.yml
.github/scripts/package-release.go
```

工作流要求：

```text
1. env.PLUGIN_ID = priority-auto-router。
2. push / pull_request / workflow_dispatch 运行 test + vet。
3. tag v* 时发布 release assets。
4. 构建 linux amd64/arm64、darwin amd64/arm64、windows amd64/arm64、freebsd amd64。
5. Archive 文件名：
   priority-auto-router_<version>_<goos>_<goarch>.zip
6. zip 根目录只放 CPA 期望的动态库：
   priority-auto-router.so / .dylib / .dll
7. 生成 checksums.txt。
```

### Task 3：实现 ABI 骨架

参考 `cpa-plugin-jshandler` 的 `abi.go` 实现基础动态库 ABI。

需要导出：

```text
cliproxy_plugin_init
PriorityAutoRouterPluginCall
PriorityAutoRouterPluginFree
PriorityAutoRouterPluginShutdown
```

需要支持 CPA ABI 方法：

```text
plugin.register
plugin.reconfigure
model.route
executor.identifier
executor.execute
executor.execute_stream
```

要求：

```text
1. 支持 JSON envelope。
2. 错误统一返回 ok=false。
3. 不 panic；必要时 recover。
4. shutdown 清理 route plan map、流资源等。
```


### Task 3.1 实现 buildPlugin 与 Metadata

`main.go` 中实现：

```text
buildPlugin(configYAML []byte, pluginDir string) (pluginapi.Plugin, error)
```

Metadata 要包含：

```text
Name: priority-auto-router
Version: pluginVersion
Author: Amin
GitHubRepository: https://github.com/router-for-me/cpa-plugin-priority-auto-router
ConfigFields:
  - client_models
  - clients
  - fallback
```

Capabilities 要包含：

```text
ModelRouter
Executor
```

字段名必须以当前 `sdk/pluginapi` 源码为准。

### Task 4：实现配置解析

实现结构：

```go
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
```

实现：

```text
loadConfig
applyDefaults
validateConfig
atomicConfigStore
```

### Task 5：实现 candidate 排序

排序规则：

```text
priority 大的优先。
priority 相同，codex_oauth 优先。
priority 和 channel_type 都相同，保持配置顺序。
```

需要测试：

```text
priority 高优先。
同 priority codex_oauth 优先。
同 priority 且都不是 codex_oauth 保持顺序。
同 priority 且都是 codex_oauth 保持顺序。
```

### Task 6：实现 model.route

逻辑：

```text
1. 解析请求。
2. 如果 header 有 X-CPA-Priority-Auto-Router-Bypass: 1，Handled=false。
3. 如果 RequestedModel 不在 client_models，Handled=false。
4. 根据 SourceFormat / User-Agent 识别 client_type。
5. 读取 client candidates。
6. 根据 AvailableProviders 过滤 candidates。
7. 排序 candidates。
8. 保存 RoutePlan。
9. 返回 Handled=true, TargetKind=self。
```

### Task 7：实现 RoutePlan 存储

优先用 SDK 提供的 metadata/context 方式。

如果没有，则用内存 TTL map：

```text
key = request_id
value = RoutePlan
ttl = 5 minutes
```

需要：

```text
1. 并发安全。
2. executor 完成后清理。
3. 定时清理过期 route plan。
```

### Task 8：实现 executor.identifier

返回：

```text
cpa-plugin-priority-auto-router
```

### Task 9：实现 executor.execute

逻辑：

```text
1. 加载 RoutePlan。
2. 遍历排序后的 candidates。
3. 调 host.model.execute。
4. 成功直接返回。
5. 失败提取 status。
6. 如果不允许 fallback，直接返回错误。
7. 如果允许 fallback，继续下一个 candidate。
8. 全部失败，返回 all candidates failed。
```

### Task 10：实现 host.model.execute 包装

包装函数：

```go
func callHostModelExecute(req ExecutorRequest, candidate Candidate) (ExecutorResponse, error)
```

要求：

```text
1. model = candidate.Model。
2. entry_protocol / exit_protocol 使用当前请求协议。
3. body 使用原始请求 body。
4. 添加 X-CPA-Priority-Auto-Router-Bypass: 1。
5. 透传 host_callback_id。
6. 不复制客户端 Authorization。
7. 不打印 body。
```

### Task 11：实现 executor.execute_stream

要求：

```text
1. 调 host.model.execute_stream。
2. 读取第一个 chunk。
3. 第一个 chunk 前失败，可以 fallback。
4. 第一个 chunk 发出后失败，不允许 fallback。
5. 每次 stream 使用后都调用 host.model.stream_close。
6. 使用 host.stream.emit / host.stream.close 输出给客户端。
```

### Task 12：实现 fallback 判断

实现：

```text
extractStatus
isNetworkError
shouldFallback
```

默认：

```text
fallback: 401, 403, 408, 409, 429, 500, 502, 503, 504
no fallback: 400, 404, 422
```

### Task 13：实现安全日志

日志函数：

```text
logRouteStart
logCandidateSuccess
logFallback
logNoFallback
logAllFailed
```

禁止日志：

```text
Authorization
Bearer
api-key
access_token
refresh_token
StorageJSON
完整请求 body
完整响应 body
```

### Task 14：测试

单元测试覆盖：

```text
配置默认值
配置校验
客户端识别
candidate 排序
AvailableProviders 过滤
fallback 判断
bypass 防递归
route plan 存取
非流式 fallback
流式首 chunk 前 fallback
流式首 chunk 后不 fallback
```

### Task 15：README

README 包含：

```text
1. 插件作用。
2. 构建方式。
3. 安装路径。
4. CPA config.example.yaml。
5. Claude Code 配置。
6. Codex CLI 配置。
7. priority 规则。
8. fallback 规则。
9. 日志安全说明。
10. 常见问题。
```

## 2. config.example.yaml

```yaml
api-keys:
  - "sk-cpa-local"

plugins:
  enabled: true
  dir: "plugins"
  configs:
    priority-auto-router:
      enabled: true
      priority: 100

      client_models:
        - "code"

      clients:
        claude:
          source_formats:
            - "claude"
            - "anthropic"
          user_agent_contains:
            - "claude"
          candidates:
            - name: "claude_api"
              channel_type: "claude_api_key"
              provider: "claude"
              model: "claude/code"
              priority: 100

            - name: "codex_oauth"
              channel_type: "codex_oauth"
              provider: "codex"
              model: "oauth-codex"
              priority: 90

        codex:
          source_formats:
            - "openai"
            - "responses"
            - "codex"
          user_agent_contains:
            - "codex"
          candidates:
            - name: "codex_api_key"
              channel_type: "codex_api_key"
              provider: "codex"
              model: "thirdcodex/code"
              priority: 100

            - name: "codex_oauth"
              channel_type: "codex_oauth"
              provider: "codex"
              model: "oauth-codex"
              priority: 90

      fallback:
        enabled: true
        fallback_on_status: [401, 403, 408, 409, 429, 500, 502, 503, 504]
        no_fallback_on_status: [400, 404, 422]
        stream_fallback_before_first_chunk_only: true

claude-api-key:
  - api-key: "你的第三方供应商 key"
    prefix: "claude"
    base-url: "https://你的第三方 Claude 协议地址"
    models:
      - name: "gpt-5-codex"
        alias: "code"

codex-api-key:
  - api-key: "你的第三方供应商 key"
    prefix: "thirdcodex"
    base-url: "https://你的第三方 Codex 协议地址"

oauth-model-alias:
  codex:
    - name: "gpt-5-codex"
      alias: "oauth-codex"
```

## 3. 给 Codex / Claude Code 的实现提示词

```text
你现在要实现一个独立 CPA 动态库插件仓库，仓库名为 cpa-plugin-priority-auto-router，运行时插件 ID、配置 key、动态库名为 priority-auto-router。

请严格按以下目标实现：

一、插件目标

实现 Claude Code CLI 和 Codex CLI 的自动路由与自动 fallback。

用户侧两个客户端都只请求同一个模型名：

  code

路由目标：

  Claude Code -> CPA -> claude-api-key + CPA Codex OAuth
  Codex CLI   -> CPA -> codex-api-key  + CPA Codex OAuth

插件根据客户端来源选择 candidates，然后按 priority 排序调用。

二、priority 规则

Candidate 结构包含：

  name
  channel_type
  provider
  model
  priority

排序规则：

  1. priority 数字越大，越优先调用。
  2. priority 相同，channel_type = codex_oauth 的候选优先。
  3. priority 和 channel_type 都相同，按配置中的 candidates 原始顺序调用。

三、插件能力

插件需要声明：

  model_router: true
  executor: true

当 model.route 识别到需要处理的请求时，返回：

  Handled=true
  TargetKind=self

因为需要 executor 自己编排 fallback。

四、model.route 逻辑

model.route 需要：

  1. 读取 RequestedModel。
  2. 只有 RequestedModel 在 client_models 中才处理，默认 code。
  3. 根据 SourceFormat / User-Agent 判断 client_type。
  4. Claude Code 命中 clients.claude。
  5. Codex CLI 命中 clients.codex。
  6. 读取对应 candidates。
  7. 根据 AvailableProviders 过滤 provider 不可用的 candidate。
  8. 按 priority 排序。
  9. 保存 RoutePlan。
  10. 返回 TargetKind=self。

如果不识别、非 code 模型、无可用 candidates，返回 Handled=false，让 CPA 原生路由继续处理。

五、executor.execute 逻辑

executor.execute 需要：

  1. 读取 RoutePlan。
  2. 按排序后的 candidates 依次调用 host.model.execute。
  3. 成功就返回。
  4. 失败则提取 status。
  5. 如果 status 在 fallback_on_status，继续下一个 candidate。
  6. 如果 status 在 no_fallback_on_status，直接返回错误。
  7. 全部 candidate 失败，返回 all candidates failed 错误。

默认 fallback_on_status：

  401, 403, 408, 409, 429, 500, 502, 503, 504

默认 no_fallback_on_status：

  400, 404, 422

网络错误、超时、连接重置默认允许 fallback。

六、executor.execute_stream 逻辑

流式请求只允许在第一个有效 chunk 返回前 fallback。

如果 primary candidate 在首 chunk 前失败，可以尝试下一个 candidate。

如果已经向客户端发送了第一个 chunk，则不能再 fallback。

每次流式 host.model.execute_stream 使用后必须 close stream。

七、host.model.* 调用

executor 中不要直接请求上游 HTTP，不要直接读取 API Key，不要直接读取 codex_oauth_xxx.json。

必须通过 CPA 宿主回调：

  host.model.execute
  host.model.execute_stream
  host.model.stream_read
  host.model.stream_close

调用时：

  model = candidate.model
  entry_protocol / exit_protocol 使用当前请求协议
  body 使用原始请求 body
  headers 添加 X-CPA-Priority-Auto-Router-Bypass: 1
  透传 host_callback_id，避免递归调用自身插件

不要复制客户端 Authorization 到 host.model.* 请求中。

八、防递归

model.route 看到以下 header 时必须返回 Handled=false：

  X-CPA-Priority-Auto-Router-Bypass: 1

executor 调 host.model.* 时必须带上该 header。

如果 SDK 支持 host_callback_id，必须透传。

九、安全要求

禁止日志输出：

  API Key
  Authorization
  Bearer Token
  access_token
  refresh_token
  codex_oauth_xxx.json 原文
  StorageJSON
  完整用户请求 body
  完整模型响应 body

允许日志输出：

  request_id
  client_type
  requested_model
  sorted_candidates
  selected_candidate
  status_code
  fallback_reason
  final_result

十、配置示例

请实现对如下配置的解析：

plugins:
  enabled: true
  dir: "plugins"
  configs:
    priority-auto-router:
      enabled: true
      priority: 100

      client_models:
        - "code"

      clients:
        claude:
          source_formats:
            - "claude"
            - "anthropic"
          user_agent_contains:
            - "claude"
          candidates:
            - name: "claude_api"
              channel_type: "claude_api_key"
              provider: "claude"
              model: "claude/code"
              priority: 100

            - name: "codex_oauth"
              channel_type: "codex_oauth"
              provider: "codex"
              model: "oauth-codex"
              priority: 90

        codex:
          source_formats:
            - "openai"
            - "responses"
            - "codex"
          user_agent_contains:
            - "codex"
          candidates:
            - name: "codex_api_key"
              channel_type: "codex_api_key"
              provider: "codex"
              model: "thirdcodex/code"
              priority: 100

            - name: "codex_oauth"
              channel_type: "codex_oauth"
              provider: "codex"
              model: "oauth-codex"
              priority: 90

      fallback:
        enabled: true
        fallback_on_status: [401, 403, 408, 409, 429, 500, 502, 503, 504]
        no_fallback_on_status: [400, 404, 422]
        stream_fallback_before_first_chunk_only: true

十一、测试

请至少添加以下测试：

  1. 非 code 模型返回 Handled=false。
  2. SourceFormat=claude 命中 claude client。
  3. User-Agent 包含 codex 命中 codex client。
  4. priority 高的 candidate 排前面。
  5. priority 相同时 codex_oauth 排前面。
  6. priority 相同且都不是 codex_oauth 时保持原顺序。
  7. AvailableProviders 过滤不可用 provider。
  8. 429 触发 fallback。
  9. 400 不触发 fallback。
  10. 网络错误触发 fallback。
  11. 流式首 chunk 前失败触发 fallback。
  12. 流式首 chunk 后失败不触发 fallback。
  13. bypass header 生效。
  14. host_callback_id 被透传。

十二、交付物

请交付：

  cpa-plugin-priority-auto-router 动态库插件源码
  README.md
  config.example.yaml
  单元测试
  构建说明

请优先参考：

  sdk/pluginapi/types.go
  sdk/pluginabi/types.go
  internal/pluginhost/model_router.go
  internal/pluginhost/host_callbacks.go
  examples/plugin/simple/go/main.go
  examples/plugin/claude-web-search-router/go/main.go
  examples/plugin/claude-web-search-router/go/fallback.go
  examples/plugin/host-model-callback/go/main.go

实现时不要臆测 SDK 字段名，先读取项目源码确认真实类型与字段。
```
