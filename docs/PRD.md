# PRD.md - CPA Plugin Priority Auto Router 自动路由插件

## 1. 项目名称

CPA Plugin Priority Auto Router

仓库名：

```text
cpa-plugin-priority-auto-router
```

运行时插件 ID / 配置 key / 动态库名：

```text
priority-auto-router
```


## 1.1 命名规范

参考 CPA 官方插件仓库风格：

```text
仓库名：cpa-plugin-priority-auto-router
运行时插件 ID：priority-auto-router
配置 key：priority-auto-router
动态库名：priority-auto-router.so / priority-auto-router.dylib / priority-auto-router.dll
Release 包名：priority-auto-router_<version>_<goos>_<goarch>.zip
```

说明：

```text
cpa-plugin-xxx 是仓库/插件项目命名。
xxx 是 CPA 加载时使用的短插件 ID、配置 key、动态库名。
```

## 2. 背景

当前希望通过 CLIProxyAPI 统一接入 Claude Code CLI 与 Codex CLI，并同时使用：

```text
Claude Code -> CPA -> claude-api-key + CPA Codex OAuth
Codex CLI   -> CPA -> codex-api-key  + CPA Codex OAuth
```

用户不希望在 Claude Code / Codex CLI 中手动切换模型 alias，而是希望两个客户端都固定请求同一个模型名：

```text
code
```

插件负责自动判断客户端来源、读取候选通道、按 priority 排序，并在主通道失败时自动 fallback。

## 3. 目标

### 3.1 总体目标

实现一个 CPA 动态库插件，使得：

```text
Claude Code -> CPA -> model=code -> 自动候选排序 -> 调用最优通道
Codex CLI   -> CPA -> model=code -> 自动候选排序 -> 调用最优通道
```

候选通道包括：

```text
claude-api-key
codex-api-key
CPA Codex OAuth
```

### 3.2 Claude Code 路由目标

Claude Code 请求 `model=code` 时，插件从 `clients.claude.candidates` 读取候选通道，例如：

```text
claude_api
codex_oauth
```

然后按 priority 排序调用。

### 3.3 Codex CLI 路由目标

Codex CLI 请求 `model=code` 时，插件从 `clients.codex.candidates` 读取候选通道，例如：

```text
codex_api_key
codex_oauth
```

然后按 priority 排序调用。

### 3.4 priority 规则

插件内部候选通道排序规则：

```text
1. priority 数字越大，越优先调用。
2. priority 相同，channel_type = codex_oauth 的候选通道优先。
3. priority 和 channel_type 都相同，按 YAML 配置中的 candidates 原始顺序调用。
```

示例：

```yaml
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
    priority: 100
```

执行顺序：

```text
1. codex_oauth
2. claude_api
```

因为 priority 相同，`codex_oauth` 优先。

## 4. 非目标

本插件不负责：

```text
1. 不负责实现新的上游模型协议。
2. 不负责解析、保存、刷新 codex_oauth_xxx.json。
3. 不直接读取或打印 API Key / OAuth Token。
4. 不替代 CPA 的凭证管理、日志、用量统计、代理设置。
5. 不做复杂 UI 管理页面。
6. 不强行处理所有模型，只处理配置中声明的 client_models，例如 code。
```

## 5. 用户侧体验

### 5.1 Claude Code 配置

用户只需要配置：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8317
export ANTHROPIC_AUTH_TOKEN=sk-cpa-local
export ANTHROPIC_DEFAULT_SONNET_MODEL=code
export ANTHROPIC_DEFAULT_OPUS_MODEL=code
export ANTHROPIC_DEFAULT_HAIKU_MODEL=code
```

### 5.2 Codex CLI 配置

用户只需要配置：

```toml
model = "code"
model_provider = "cliproxyapi"
model_reasoning_effort = "high"

[model_providers.cliproxyapi]
name = "cliproxyapi"
base_url = "http://127.0.0.1:8317/v1"
wire_api = "responses"
experimental_bearer_token = "sk-cpa-local"
requires_openai_auth = true
```

## 6. 推荐 CPA 配置

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

## 7. priority 使用示例

### 7.1 默认 API Key 优先，OAuth 兜底

Claude Code：

```yaml
- name: "claude_api"
  channel_type: "claude_api_key"
  model: "claude/code"
  priority: 100

- name: "codex_oauth"
  channel_type: "codex_oauth"
  model: "oauth-codex"
  priority: 90
```

效果：

```text
Claude Code -> claude_api -> 失败后 -> codex_oauth
```

Codex CLI：

```yaml
- name: "codex_api_key"
  channel_type: "codex_api_key"
  model: "thirdcodex/code"
  priority: 100

- name: "codex_oauth"
  channel_type: "codex_oauth"
  model: "oauth-codex"
  priority: 90
```

效果：

```text
Codex CLI -> codex_api_key -> 失败后 -> codex_oauth
```

### 7.2 OAuth 和 API Key 同优先级时，OAuth 优先

```yaml
- name: "claude_api"
  channel_type: "claude_api_key"
  model: "claude/code"
  priority: 100

- name: "codex_oauth"
  channel_type: "codex_oauth"
  model: "oauth-codex"
  priority: 100
```

效果：

```text
Claude Code -> codex_oauth -> 失败后 -> claude_api
```

## 8. fallback 规则

允许 fallback 的状态码：

```text
401, 403, 408, 409, 429, 500, 502, 503, 504
```

不允许 fallback 的状态码：

```text
400, 404, 422
```

说明：

```text
400 / 404 / 422 多数是请求格式、模型名、参数问题，fallback 大概率无效，所以默认不 fallback。
401 / 403 可能是认证、权限、额度问题，可以 fallback。
429 是限流或额度不足，可以 fallback。
5xx 是上游异常，可以 fallback。
网络超时、连接重置、DNS 失败等网络错误可以 fallback。
```

## 9. 流式请求规则

流式请求只允许在第一个有效 chunk 返回前 fallback。

如果主通道已经向客户端输出了任何有效 chunk，则不允许再切换到下一个通道，避免客户端收到混合响应。

## 10. 日志要求

允许日志字段：

```text
request_id
client_type
requested_model
stream
sorted_candidates
selected_candidate
candidate_priority
fallback_reason
status_code
final_result
```

禁止打印：

```text
API Key
Authorization
Bearer Token
access_token
refresh_token
codex_oauth_xxx.json 原文
完整用户请求 body
完整模型响应 body
```


## 11. 工程与发布要求

插件应采用独立仓库结构，而不是放在 CLIProxyAPI 的 examples 目录中。

建议：

```text
repository: router-for-me/cpa-plugin-priority-auto-router
module: github.com/router-for-me/cpa-plugin-priority-auto-router
runtime plugin id: priority-auto-router
```

构建产物：

```text
linux/freebsd: priority-auto-router.so
darwin: priority-auto-router.dylib
windows: priority-auto-router.dll
```

Release archive 命名：

```text
priority-auto-router_0.1.0_linux_amd64.zip
priority-auto-router_0.1.0_linux_arm64.zip
priority-auto-router_0.1.0_darwin_amd64.zip
priority-auto-router_0.1.0_darwin_arm64.zip
priority-auto-router_0.1.0_windows_amd64.zip
priority-auto-router_0.1.0_windows_arm64.zip
checksums.txt
```

README 必须包含：

```text
Features
Configuration
Fields
Building
Plugin Store Release Assets
Examples
Troubleshooting
```


## 12. 验收标准

### Case 1：Claude Code，API Key 优先

配置：

```text
claude_api.priority = 100
codex_oauth.priority = 90
```

预期：

```text
Claude Code -> claude_api
```

### Case 2：Claude Code，priority 相同 OAuth 优先

配置：

```text
claude_api.priority = 100
codex_oauth.priority = 100
```

预期：

```text
Claude Code -> codex_oauth
```

### Case 3：Codex CLI，API Key 优先

配置：

```text
codex_api_key.priority = 100
codex_oauth.priority = 90
```

预期：

```text
Codex CLI -> codex_api_key
```

### Case 4：Codex CLI，priority 相同 OAuth 优先

配置：

```text
codex_api_key.priority = 100
codex_oauth.priority = 100
```

预期：

```text
Codex CLI -> codex_oauth
```

### Case 5：主通道失败自动 fallback

输入：

```text
client = Codex CLI
model = code
codex_api_key 返回 429
codex_oauth 正常
```

预期：

```text
Codex CLI -> codex_api_key failed -> codex_oauth success
```

### Case 6：非 code 模型不拦截

输入：

```text
model = claude/code
```

预期：

```text
插件返回 Handled=false，由 CPA 原生路由处理。
```
