# OpenAI Responses API — implementation reference

This is implementation-grade reference data for the OpenAI Responses
API surface ikigai-cli will call directly over HTTPS. The high-level
requirements (which models to support, effort vocabulary, auth
conventions, etc.) live in `../providers.md`. This file pins the
wire-level shapes the build agent needs to construct requests and
parse responses.

Verified against current docs at `developers.openai.com` as of
2026-05-09.

## 1. Endpoint and auth

- **Method / URL**: `POST https://api.openai.com/v1/responses`
  (same path for streaming and non-streaming; streaming selected
  by request body field `stream: true`).
- **Required headers** (verbatim):
  ```
  Authorization: Bearer <OPENAI_API_KEY>
  Content-Type: application/json
  ```
- **Auth header**: `Authorization: Bearer <key>` (NOT `x-api-key`).
  Read the key from env var `OPENAI_API_KEY`; pass its value as the
  bearer credential.
- **No org / project routing in v1.** `OpenAI-Organization` and
  `OpenAI-Project` headers are not sent. Azure OpenAI is not
  supported.

[Responses API](https://developers.openai.com/api/docs/api-reference/responses) ·
[Authentication](https://developers.openai.com/api/docs/api-reference/authentication)

## 2. Request body shape

Top-level fields ikigai-cli sends (✓ = required):

- `model`✓ — bare API ID, e.g. `"gpt-5.5"`.
- `input`✓ — string OR array of input items (see §2.1).
- `instructions` — system-prompt string, applied as the
  highest-priority developer message. Equivalent to a `system`/
  `developer` role item at the top of `input`.
- `tools` — array of tool definitions (see §3).
- `tool_choice` — `"auto"` (default when tools present) | `"none"`
  | `"required"` | `{"type":"function","name":"<tool>"}`.
- `reasoning` — `{"effort":"none|low|medium|high|xhigh","summary":"auto|concise|detailed|none"}`.
  Effort vocabulary is per-model; see `../providers.md` for legal
  values per model.
- `text.format` — structured-output spec; see §6.
- `store` — boolean. ikigai-cli sends `false` (stateless).
- `include` — array of strings selecting optional fields in the
  response. ikigai-cli sends `["reasoning.encrypted_content"]`
  whenever a reasoning model is in use (see §5).
- `stream` — boolean. ikigai-cli sends `true`.
- `max_output_tokens` — integer, optional cap on generated tokens
  (counts against reasoning + visible output combined).

### 2.1 Input item shapes

`input` is either a plain string (treated as a single user message)
or an array of typed items in conversation order:

```json
// user / assistant / developer message
{"type":"message","role":"user|assistant|developer","content":[
  {"type":"input_text","text":"..."},
  {"type":"input_image","image_url":"..."},
  {"type":"input_file","file_id":"..."}
]}

// assistant text echoed back
{"type":"message","role":"assistant","content":[
  {"type":"output_text","text":"..."}
]}

// reasoning item — round-trip unchanged (see §5)
{"type":"reasoning","id":"rs_...","summary":[],
 "encrypted_content":"<opaque-string>"}

// model-emitted function call — round-trip unchanged
{"type":"function_call","id":"fc_...","call_id":"call_...",
 "name":"get_weather","arguments":"{\"location\":\"Paris\"}"}

// caller's reply to a function_call
{"type":"function_call_output","call_id":"call_...","output":"15"}
```

`role: "developer"` outranks `role: "system"` in the Responses
API; `instructions` is sugar for prepending a developer message.

[Create response](https://developers.openai.com/api/docs/api-reference/responses/create)

## 3. Tool definition shape

```json
{
  "type": "function",
  "name": "get_weather",
  "description": "Retrieves current weather for the given location.",
  "parameters": {
    "type": "object",
    "properties": {
      "location": {"type": "string"},
      "units": {"type": ["string","null"], "enum": ["celsius","fahrenheit"]}
    },
    "required": ["location","units"],
    "additionalProperties": false
  },
  "strict": true
}
```

When `strict: true`:
- `parameters` must set `additionalProperties: false` on every
  object level.
- `required` must list every property declared in `properties`
  (optional fields are expressed via `["string","null"]` union
  types, not by omission from `required`).

`tool_choice` shapes:
```json
"tool_choice": "auto"                                       // default with tools
"tool_choice": "none"
"tool_choice": "required"
"tool_choice": {"type":"function","name":"get_weather"}     // force one tool
```

[Function calling](https://developers.openai.com/api/docs/guides/function-calling)

## 4. Streaming SSE format

Events ikigai-cli must handle. Each SSE frame has both an `event:`
line and a `type` field in `data:` matching the event name.

Lifecycle events (emitted once each per response):
- `response.created` — `data` carries the initial `response`
  object with empty `output`.
- `response.in_progress` — generation has begun.
- `response.completed` — terminal event on success; `data.response`
  carries the final `output` array and `usage` totals.
- `response.failed` — terminal event on model-side failure;
  `data.response.error` carries the failure shape.
- `error` — transport-level error frame; same body shape as a
  non-streaming HTTP error response (§7).

Per-output-item events (emitted as items are added to
`response.output[]`):
- `response.output_item.added` — a new top-level output item
  begins. `data.item` is one of `{type: "message"}`,
  `{type: "reasoning"}`, `{type: "function_call"}`, etc.
- `response.output_item.done` — that item is finalized.

Per-content-part events (within a `message` item):
- `response.content_part.added` — a new content part within a
  message item.
- `response.content_part.done` — that content part is finalized.

Text streaming (within an `output_text` content part):
- `response.output_text.delta` — incremental text chunk.
  `data.delta` is the string fragment.
- `response.output_text.done` — terminal event for this content
  part; `data.text` is the assembled string.

Function-call streaming (within a `function_call` item):
- `response.function_call_arguments.delta` — incremental JSON
  string fragment for the call's `arguments` field. Accumulate
  raw, then `JSON.parse` on `done`.
- `response.function_call_arguments.done` — terminal event; the
  full arguments string is now available on the parent
  `function_call` item.

Reasoning streaming (within a `reasoning` item, only when
`reasoning.summary` is enabled):
- `response.reasoning_summary_text.delta` — incremental summary
  text chunk.
- `response.reasoning_summary_text.done` — terminal event for
  this summary part.

The raw chain-of-thought is **not** exposed on the wire; only the
optional summary is human-readable. The signed reasoning state
arrives on `response.completed` / `response.output_item.done` as
the `encrypted_content` field of the `reasoning` item — that's
what gets round-tripped per §5.

[Streaming events](https://developers.openai.com/api/docs/api-reference/responses-streaming)

## 5. Reasoning preservation (stateless)

ikigai-cli runs in stateless mode: every request sets
`store: false`. To carry the model's signed reasoning state across
tool-use round-trips within a single iteration:

1. Every request that may produce reasoning sends
   `include: ["reasoning.encrypted_content"]`.
2. On `response.completed`, the final `output` array contains
   `reasoning` items with non-empty `encrypted_content` strings.
3. The next request in the same iteration's `input` array carries
   those `reasoning` items **unchanged** (same `id`, same
   `encrypted_content`), interleaved with the new
   `function_call_output` reply in original arrival order.

Dropping or modifying a `reasoning` item between turns invalidates
the signature; subsequent requests fail. This is OpenAI's
equivalent of Anthropic's signed thinking blocks (see
`anthropic.md` §5).

[Reasoning models](https://developers.openai.com/api/docs/guides/reasoning)

## 6. Structured output

```json
"text": {
  "format": {
    "type": "json_schema",
    "name": "<short_name>",
    "strict": true,
    "schema": { /* JSON Schema object */ }
  }
}
```

Constraints when `strict: true` (enforced by OpenAI; violating any
of these yields a 400 at request time):

- `additionalProperties: false` on every object schema in the tree.
- `required` must list every property declared in `properties`.
  Optional fields are expressed as union types with `null`, not by
  omission.
- Recursive schemas use `{"$ref": "#"}` for self-reference.

The constrained output appears as an `output_text` content part on
the final assistant `message` item. The `text` field of that part
is parsable JSON conforming to the schema.

ikigai-cli always sets `strict: true`. The schema passed via
`--json-schema` (per OVERVIEW R-JNEB-EVLU) is forwarded verbatim
into `text.format.schema`; if the supplied schema violates the
strict-mode constraints above, the resulting 400 is surfaced to
the operator per R-E2W7-K5JB without translation — repairing the
schema is the operator's responsibility.

[Structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs)

## 7. Error response shape

Non-streaming and pre-stream HTTP errors (status 4xx/5xx with JSON
body):

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "...",
    "param": "...",
    "message": "..."
  }
}
```

Common `error.type` values:
- `invalid_request_error` (400)
- `authentication_error` (401)
- `permission_error` (403)
- `not_found_error` (404)
- `rate_limit_error` (429)
- `server_error` (500)
- `overloaded_error` (503-equivalent)

In-stream errors arrive as an SSE `event: error` frame after the
HTTP 200 has been emitted, or as `response.failed` with
`response.error` populated.

ikigai-cli classifies these into the `provider.ErrorKind` taxonomy
(`ErrAuth`, `ErrInvalidRequest`, `ErrRateLimit`, `ErrTimeout`,
`ErrServer`, `ErrUnknown`) and surfaces them per R-E2W7-K5JB. Raw
HTTP status codes and response bodies do not leak onto stdout.

[Errors](https://developers.openai.com/api/docs/api-reference/errors)

## 8. Usage object

`response.completed` carries cumulative usage:

```json
"usage": {
  "input_tokens": 25,
  "input_tokens_details": {"cached_tokens": 0},
  "output_tokens": 503,
  "output_tokens_details": {"reasoning_tokens": 200},
  "total_tokens": 528
}
```

ikigai-cli maps these to `provider.EventUsage`:
- `InputTokens` ← `usage.input_tokens`
- `OutputTokens` ← `usage.output_tokens` (includes reasoning
  tokens; matches OpenAI's billing semantics)
- `CacheReadInputTokens` and `CacheCreationInputTokens` ← 0
  (Anthropic-only fields per R-1TGL-373X; OpenAI's
  `cached_tokens` is not exposed on the iteration result event in
  v1).

## Notes / unconfirmed

- gpt-5.5 specifically does NOT accept `reasoning.effort:
  "minimal"` (despite that value being legal on some other 5.x
  models). Verified against the gpt-5.5 model page on
  developers.openai.com 2026-05-09.
- The exact mapping between `reasoning.effort` values and internal
  reasoning-token budgets is not published — treat as opaque.
- `response.reasoning_summary_*` events are only emitted when
  `reasoning.summary` is requested. ikigai-cli does not request
  summaries in v1 (see `../providers.md` R-4JYG-IMBI); the events
  are documented here for completeness.
