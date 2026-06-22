# CPA Priority Auto Router Plugin

`priority-auto-router` is an independent CLIProxyAPI (CPA) dynamic-library plugin that lets clients request one model alias, usually `code`, while CPA routes the request through prioritized provider candidates and fallback rules.

The plugin uses CPA `model_router`, `executor`, and `host.model.*` callbacks. It does not make direct upstream HTTP calls and does not read API keys, OAuth tokens, or local OAuth JSON files.

## Features

- Handles only configured client-facing model names; default target is `code`.
- Detects Claude Code and Codex CLI by source format first, then User-Agent substring.
- Routes Claude Code through `clients.claude.candidates` and Codex CLI through `clients.codex.candidates`.
- Sorts candidates by priority descending, then `channel_type: codex_oauth` first on equal priority, then original config order.
- Filters candidates by CPA `available_providers` when CPA supplies that list.
- Calls CPA host model callbacks with `X-CPA-Priority-Auto-Router-Bypass: 1` and forwards `host_callback_id` to avoid recursive plugin routing.
- Sanitizes sensitive client headers before host model execution.
- Supports non-stream fallback and stream fallback before the first emitted chunk.

## Configuration

Install the compiled dynamic library as the CPA plugin ID/config key `priority-auto-router`, then configure it with YAML like `config.example.yaml`.

Minimal shape:

```yaml
client_models: [code]
clients:
  claude:
    source_formats: [claude, anthropic]
    user_agent_contains: [claude]
    candidates:
      - name: claude_api
        channel_type: claude_api_key
        provider: claude
        model: claude/code
        priority: 100
  codex:
    source_formats: [codex, openai, responses, openai-response]
    user_agent_contains: [codex]
    candidates:
      - name: codex_api_key
        channel_type: codex_api_key
        provider: codex
        model: thirdcodex/code
        priority: 100
fallback:
  enabled: true
```

If `client_models` or `fallback` fields are omitted, defaults are applied. `clients` and each client's `candidates` are required.

## Fields

### `client_models`

List of client-facing model names this plugin handles. Defaults to:

```yaml
client_models: [code]
```

Requests for other model names return `Handled=false` and continue through CPA native routing.

### `clients`

Map of client profiles. Common profile names are `claude` and `codex`.

Each profile supports:

- `source_formats`: source protocol names used for client detection. Aliases such as `anthropic`, `responses`, and `chat-completions` are normalized internally.
- `user_agent_contains`: case-insensitive User-Agent substrings used when source format does not identify a client.
- `candidates`: prioritized provider targets.

Candidate fields:

- `name`: unique candidate name within the client profile.
- `channel_type`: one of `claude_api_key`, `codex_api_key`, or `codex_oauth`.
- `provider`: CPA provider ID to route through, for example `claude` or `codex`.
- `model`: upstream/provider-facing model name passed to `host.model.*`.
- `priority`: higher numbers are tried first.

Ordering rule: higher `priority` first; on equal priority, `codex_oauth` first; on equal priority and channel type, YAML order is preserved.

### `fallback`

Fallback defaults:

```yaml
fallback:
  enabled: true
  fallback_on_status: [401, 403, 408, 409, 429, 500, 502, 503, 504]
  no_fallback_on_status: [400, 404, 422]
  stream_fallback_before_first_chunk_only: true
```

Non-stream requests try the next candidate for configured fallback statuses and network-like errors. Terminal statuses such as `400`, `404`, and `422` do not fallback.

Streaming requests may fallback only before the first downstream chunk has been emitted. After a chunk is emitted, errors close the stream instead of switching candidates.

## Building

Requirements:

- Go matching the module version in `go.mod`.
- A platform that supports Go `-buildmode=c-shared`.

Build for the host platform:

```bash
make build
```

Expected dynamic library names:

- Linux/FreeBSD: `priority-auto-router.so`
- macOS: `priority-auto-router.dylib`
- Windows: `priority-auto-router.dll`

Run local checks when Go is available:

```bash
go test ./...
go vet ./...
make build
```

## Plugin Store Release Assets

The release workflow packages archives named:

```text
priority-auto-router_<version>_<goos>_<goarch>.zip
```

Each archive contains the dynamic library at the archive root using the expected platform name, such as `priority-auto-router.dylib` on macOS or `priority-auto-router.dll` on Windows. Release checksums are emitted as `checksums.txt`.

## Examples

### Claude Code prefers Claude API key, then Codex OAuth

```yaml
clients:
  claude:
    source_formats: [claude, anthropic]
    user_agent_contains: [claude]
    candidates:
      - name: claude_api
        channel_type: claude_api_key
        provider: claude
        model: claude/code
        priority: 100
      - name: codex_oauth
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
        priority: 90
```

With equal priority, `codex_oauth` wins the tie:

```yaml
priority: 100
```

on both candidates routes `codex_oauth` first.

### Codex CLI prefers Codex API key, then Codex OAuth

```yaml
clients:
  codex:
    source_formats: [codex, openai, responses, openai-response]
    user_agent_contains: [codex]
    candidates:
      - name: codex_api_key
        channel_type: codex_api_key
        provider: codex
        model: thirdcodex/code
        priority: 100
      - name: codex_oauth
        channel_type: codex_oauth
        provider: codex
        model: oauth-codex
        priority: 90
```

### Single model alias

Configure Claude Code and Codex CLI to request:

```text
code
```

The plugin decides which candidate to try based on the detected client and fallback state.

## Troubleshooting

- Request is not handled: confirm the requested model is listed in `client_models` and no bypass header is present.
- Wrong client profile is selected: check `source_formats` first, then `user_agent_contains`; source format takes precedence.
- No candidate is tried: verify the CPA provider is available and matches each candidate's `provider` value.
- Fallback does not happen: check `fallback.enabled`, `fallback_on_status`, and `no_fallback_on_status`.
- Stream did not fallback after output began: this is expected; stream fallback is allowed only before the first emitted chunk.
- Recursive routing: the plugin sets `X-CPA-Priority-Auto-Router-Bypass: 1` and forwards CPA `host_callback_id`; ensure the CPA version supports host model callbacks.
- Credential concerns: this plugin never reads provider secrets, OAuth token files, or `StorageJSON`; credentials remain managed by CPA providers.
