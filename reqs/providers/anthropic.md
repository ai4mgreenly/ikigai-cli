# Anthropic Messages API — implementation reference

This is implementation-grade reference data for the Anthropic
Messages API surface ikigai-cli will call directly over HTTPS. The
high-level requirements (which models to support, effort vocabulary,
auth conventions, etc.) live in `../providers.md`. This file pins
the wire-level shapes the build agent needs to construct requests
and parse responses.

Verified against current docs at `platform.claude.com` as of
2026-05-08 (`docs.claude.com` and `docs.anthropic.com` 301 here).

## 1. Endpoint and auth

- **Method / URL**: `POST https://api.anthropic.com/v1/messages`
  (same path for streaming and non-streaming; streaming selected
  by request body field `stream: true`).
- **Required headers** (verbatim):
  ```
  x-api-key: <API_KEY>
  anthropic-version: 2023-06-01
  content-type: application/json
  ```
- **Auth header**: `x-api-key` (NOT `Authorization: Bearer`). Read
  the key from env var `ANTHROPIC_API_KEY`; pass its value as the
  header.
- **`anthropic-beta`** (comma-separated, additive — preserve any
  existing value, append, don't overwrite):
  - `context-1m-2025-08-07` — 1M context window (see §7).
  - `interleaved-thinking-2025-05-14` — interleaved thinking
    (deprecated on Opus 4.6/Sonnet 4.6; automatic on Opus 4.7).
  - Prompt caching needs **no** beta header (now GA).
  - Structured outputs needs **no** beta header (legacy
    `structured-outputs-2025-11-13` still works during transition).

[Messages API](https://platform.claude.com/docs/en/api/messages) ·
[Errors / request size](https://platform.claude.com/docs/en/api/errors)

## 2. Request body shape

Top-level fields (✓ = required): `model`✓, `messages`✓,
`max_tokens`✓, `system`, `temperature` (default 1.0), `top_p`,
`top_k`, `tools`, `tool_choice`, `stream`, `stop_sequences`,
`thinking`, `metadata`, `output_config`, `service_tier`.

`messages` is an array of `{role: "user"|"assistant", content:
string | ContentBlock[]}`. Consecutive same-role turns are auto-
merged. `system` is top-level (no `system` role); accepts string or
`TextBlockParam[]`.

Content block JSON shapes (request side):

```json
// text
{"type":"text","text":"...","cache_control":{"type":"ephemeral","ttl":"5m"}}

// image
{"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}
{"type":"image","source":{"type":"url","url":"https://..."}}

// document (PDF or text)
{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"..."},"title":"...","citations":{"enabled":true}}

// tool_use (assistant-side; you echo this back unchanged)
{"type":"tool_use","id":"toolu_...","name":"get_weather","input":{"location":"SF"}}

// tool_result (user-side, replying to a tool_use)
{"type":"tool_result","tool_use_id":"toolu_...","content":"string or [blocks]","is_error":false}

// thinking (echo back unchanged, including signature, on the next turn when carrying tool_use)
{"type":"thinking","thinking":"...","signature":"..."}
```

[Messages API](https://platform.claude.com/docs/en/api/messages)

## 3. Tool definition shape

```json
{
  "name": "get_weather",                       // ^[a-zA-Z0-9_-]{1,64}$
  "description": "...",
  "input_schema": { "type":"object", "properties":{...}, "required":[...] },
  "strict": true,                              // optional: constrained decoding
  "cache_control": {"type":"ephemeral"},       // optional, on last tool only
  "input_examples": [ {...} ]                  // optional
}
```

`tool_choice` shapes:
```json
{"type":"auto", "disable_parallel_tool_use": false}   // default if tools provided
{"type":"any",  "disable_parallel_tool_use": false}   // must call some tool
{"type":"tool", "name":"get_weather"}                  // must call this tool
{"type":"none"}                                        // no tool calls
```
With extended thinking, only `auto` and `none` are supported.
[Define tools](https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools)

## 4. Streaming SSE format

Event order: `message_start` → (per content block:
`content_block_start`, N×`content_block_delta`,
`content_block_stop`) → `message_delta` → `message_stop`. `ping`
events may appear anywhere. Each SSE frame has both an `event:`
line and a `type` field in `data:`.

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_...","type":"message","role":"assistant","content":[],"model":"claude-opus-4-7","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":25,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
// for tool_use blocks: content_block:{"type":"tool_use","id":"toolu_...","name":"...","input":{}}
// for thinking blocks: content_block:{"type":"thinking","thinking":"","signature":""}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}
```

Delta types:
- `text_delta` (`{text}`)
- `input_json_delta` (`{partial_json}` — string fragments;
  accumulate then `JSON.parse` on `content_block_stop`; final
  `tool_use.input` is always an object)
- `thinking_delta` (`{thinking}`)
- `signature_delta` (`{signature}`, sent once just before
  `content_block_stop` of a thinking block)

When `display:"omitted"`, a thinking block emits no
`thinking_delta` — only `content_block_start`, one
`signature_delta`, `content_block_stop`. Token counts in
`message_delta.usage` are **cumulative**.

Errors mid-stream:
```
event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}
```
[Streaming](https://platform.claude.com/docs/en/api/messages-streaming)

## 5. Extended thinking / effort

Two shapes coexist; per-model rules matter:

```json
// Manual (legacy)
"thinking": {"type":"enabled","budget_tokens":10000,"display":"summarized"|"omitted"}

// Adaptive (recommended, paired with effort)
"thinking": {"type":"adaptive"}
"effort": "low"|"medium"|"high"|"xhigh"|"max"
```

| Model | Manual `enabled` | Adaptive | Notes |
|---|---|---|---|
| Opus 4.7 | **400 error** | required | `display` defaults `omitted`; interleaved thinking automatic |
| Opus 4.6 | deprecated (works) | recommended | |
| Sonnet 4.6 | deprecated (works) | recommended | |
| Haiku 4.5 | works | recommended | |

`budget_tokens` minimum 1024 and must be `< max_tokens`. Cannot
combine with `max_tokens: 0`.

**Critical**: thinking blocks always carry a `signature` and
**must** be returned unmodified alongside their `tool_use` when
sending the next `tool_result` turn — the API verifies signatures
cryptographically. If ikigai-cli does not forward thinking to its
own callers, it must still preserve and re-send these blocks in
the conversation history sent to Anthropic.

[Extended thinking](https://platform.claude.com/docs/en/build-with-claude/extended-thinking) ·
[Context windows](https://platform.claude.com/docs/en/build-with-claude/context-windows)

## 6. Prompt caching

```json
"cache_control": {"type":"ephemeral", "ttl":"5m"|"1h"}   // ttl optional, default 5m
```
Placeable on: any content block in `system`, in
`messages[].content`, or on a tool definition (typically the last).
Up to 4 cache breakpoints per request. **No beta header required**
(GA). Cache hits surface in `usage`:
```json
"usage": {
  "input_tokens": 50,                   // tokens AFTER last cache breakpoint
  "cache_read_input_tokens": 100000,    // 0 if no hit; always present on caching-eligible models
  "cache_creation_input_tokens": 0,
  "output_tokens": 503,
  "cache_creation": {"ephemeral_5m_input_tokens": 456, "ephemeral_1h_input_tokens": 100}
}
```
`total_input = cache_read + cache_creation + input_tokens`.
[Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)

## 7. 1M context window

- **Beta header**: `anthropic-beta: context-1m-2025-08-07`. Without
  it, requests >200k input tokens 400-error.
- **Models**: Opus 4.7, Opus 4.6, Sonnet 4.6. Sonnet 4.5 and below
  remain 200k.
- **Pricing**: long-context tier applies once a single request
  exceeds 200k input tokens.
- Single request cap: 600 images/PDF pages on 1M models (100 on
  200k models); 32 MB request body.

[Context windows](https://platform.claude.com/docs/en/build-with-claude/context-windows)

## 8. Structured output

Anthropic now has **native** structured output via
`output_config.format` (was beta `output_format` +
`structured-outputs-2025-11-13`; both still work transitionally):
```json
"output_config": {
  "format": {
    "type": "json_schema",
    "schema": {"type":"object","properties":{...},"required":[...],"additionalProperties":false}
  }
}
```
Result: schema-conformant JSON in `response.content[0].text`
(constrained decoding, no parse retries needed).

**Tool-calling pattern** (still canonical for "force a structured
side-channel" or pre-native models): define a tool whose
`input_schema` is the desired shape, set `tool_choice:
{"type":"tool","name":"<tool>"}`, add `strict: true`, then read
the `tool_use.input` object from the response.
[Structured outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)

## 9. Usage object

Appears in `message_start.message.usage` (input + initial output
tokens) and `message_delta.usage` (cumulative output, plus final
cache totals). Fields:
- `input_tokens` — uncached input AFTER last cache breakpoint
- `output_tokens` — cumulative in streaming
- `cache_read_input_tokens` — present on caching-eligible models
  even when 0
- `cache_creation_input_tokens` — same
- `cache_creation: {ephemeral_5m_input_tokens,
  ephemeral_1h_input_tokens}` — only when 1h TTL is used

[Streaming](https://platform.claude.com/docs/en/api/messages-streaming) ·
[Rate limits](https://platform.claude.com/docs/en/api/rate-limits)

## 10. Error response shape

HTTP codes (with `error.type` value):
- 400 `invalid_request_error`
- 401 `authentication_error`
- 402 `billing_error`
- 403 `permission_error`
- 404 `not_found_error`
- 413 `request_too_large` (32 MB cap, returned by Cloudflare)
- 429 `rate_limit_error`
- 500 `api_error`
- 504 `timeout_error`
- 529 `overloaded_error`

Body shape (always JSON, even on 5xx; `request_id` mirrors the
`request-id` response header):
```json
{"type":"error","error":{"type":"rate_limit_error","message":"..."},"request_id":"req_..."}
```

Streaming errors arrive as an `event: error` SSE frame (same
shape, no `request_id`) after a 200 has been emitted.

**Rate-limit response headers** (returned on every response;
`retry-after` only on 429):
- `retry-after` (seconds)
- `anthropic-ratelimit-requests-{limit,remaining,reset}` (reset =
  RFC 3339)
- `anthropic-ratelimit-tokens-{limit,remaining,reset}` (most-
  restrictive aggregate)
- `anthropic-ratelimit-input-tokens-{limit,remaining,reset}`
- `anthropic-ratelimit-output-tokens-{limit,remaining,reset}`
- `anthropic-priority-input-tokens-{limit,remaining,reset}` and
  `anthropic-priority-output-tokens-{limit,remaining,reset}`
  (Priority Tier only)

`*-remaining` for tokens is rounded to the nearest thousand.
[Errors](https://platform.claude.com/docs/en/api/errors) ·
[Rate limits](https://platform.claude.com/docs/en/api/rate-limits)

## Notes / unconfirmed

- Per-model exact mapping of `effort` values (`low`/`medium`/
  `high`/`xhigh`/`max`) to internal token budgets is not
  published — treat as opaque.
- The `output_config.format` field name is current as of docs
  fetched 2026-05-08; the legacy `output_format` parameter and
  `structured-outputs-2025-11-13` beta header continue to work
  during the transition.
