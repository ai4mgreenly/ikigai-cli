# OpenAI Responses API — implementation reference

This is implementation-grade reference data for the OpenAI
Responses API surface ikigai-cli will call directly over HTTPS. The
high-level requirements (which models to support, effort vocabulary,
auth conventions, etc.) live in `../providers.md`. This file pins
the wire-level shapes the build agent needs to construct requests
and parse responses.

ikigai-cli uses the **Responses API**, not Chat Completions.
Verified against `developers.openai.com` and `platform.openai.com`
docs as of 2026-05-08.

## 1. Endpoint and auth

- **Method / URL**: `POST https://api.openai.com/v1/responses`
- **Streaming**: same endpoint; set `"stream": true` in the body.
  Response is `text/event-stream` (SSE).
- **Auth header**: `Authorization: Bearer $OPENAI_API_KEY`
- **Content-Type**: `application/json`
- **`OpenAI-Beta` header**: **not required** for the Responses API
  in current docs (it is GA). Do not send one.
- **Query params**: none for create.
- **Optional org/project headers**: `OpenAI-Organization`,
  `OpenAI-Project` (not required for ikigai-cli).

[create endpoint](https://developers.openai.com/api/reference/resources/responses/methods/create) ·
[streaming guide](https://developers.openai.com/api/docs/guides/streaming-responses)

## 2. Request body shape

Top-level fields (all optional except `model`; `input` is
effectively required for any useful call):

| Field | Type | Notes |
|---|---|---|
| `model` | string | e.g. `"gpt-5.5"` |
| `input` | string \| array of input items | see below |
| `instructions` | string | system-level guidance, prepended each turn |
| `tools` | array | see §3 |
| `tool_choice` | string \| object | see §3 |
| `stream` | bool | enable SSE |
| `max_output_tokens` | int | hard cap on output (incl. reasoning tokens) |
| `text` | object | `{ "format": { ... } }` — structured output lives here, NOT `response_format` |
| `reasoning` | object | `{ "effort": ..., "summary": ... }` — see §5 |
| `temperature` | number | not honored on reasoning models |
| `store` | bool | persist response for 30 days; default `true` |
| `previous_response_id` | string | chain on a stored prior response |
| `truncation` | string | `"auto"` or `"disabled"` (default `"disabled"`) |
| `conversation` | string \| object | server-managed conversation |
| `background` | bool | async/background generation |
| `include` | array | extra outputs (e.g. `reasoning.encrypted_content`) |

**Input item types** (each item is one of):

```json
// message
{ "type": "message",
  "role": "user|assistant|system|developer",
  "content": "string" | [ { "type": "input_text", "text": "..." } ] }

// function_call (typically echoed back from a prior assistant turn)
{ "type": "function_call",
  "id": "fc_...",          // optional on input
  "call_id": "call_...",
  "name": "tool_name",
  "arguments": "<json string>" }

// function_call_output (the tool result you return)
{ "type": "function_call_output",
  "call_id": "call_...",
  "output": "<string>" }

// reasoning (encrypted reasoning item, when carrying state stateless)
{ "type": "reasoning",
  "id": "rs_...",
  "encrypted_content": "...",
  "summary": [] }
```

**History supply**: Either resend the full input array each call,
or use `previous_response_id` chaining when `store: true`. For
ikigai-cli's stream-json model, full-input resend with
`store: false` is the natural fit (see §9).

[create](https://developers.openai.com/api/reference/resources/responses/methods/create) ·
[conversation state](https://developers.openai.com/api/docs/guides/conversation-state)

## 3. Tool definitions

Function tool entry. Note: `name`/`description`/`parameters`/
`strict` are **flat on the tool object** in the Responses API —
not nested under `function:` like Chat Completions:

```json
{
  "type": "function",
  "name": "get_weather",
  "description": "Get current weather for a city.",
  "parameters": {
    "type": "object",
    "properties": { "city": { "type": "string" } },
    "required": ["city"],
    "additionalProperties": false
  },
  "strict": true
}
```

`tool_choice`:

```json
"auto" | "none" | "required"
```

Force a specific function:

```json
{ "type": "function", "name": "get_weather" }
```

Built-in tool types exist (`file_search`, `code_interpreter`,
`web_search`, `computer_use`, plus a `custom` grammar type) —
ikigai-cli will not emit them, only pass through `function`.

[function calling](https://developers.openai.com/api/docs/guides/function-calling)

## 4. Streaming SSE events

SSE frames look like `event: <name>\ndata: <json>\n\n`. Every
payload carries `sequence_number`; deltas must be applied in order.
Events surfaced by the Responses API:

**Lifecycle envelopes** — `response.queued`, `response.created`,
`response.in_progress`, `response.completed`, `response.incomplete`,
`response.failed`, plus out-of-band `error`. Each carries
`{ response, sequence_number }` (or
`{ code, message, param, sequence_number }` for `error`).
`response.incomplete` includes `response.incomplete_details.reason`
(`"max_output_tokens"`, `"content_filter"`, ...). Read final
`usage` only from `response.completed`.

**Output item assembly** — `response.output_item.added` / `.done`
carry `{ output_index, item, sequence_number }`; `item.id` is
`msg_...`, `fc_...`, `rs_...`, etc. Create per-`item.id` buffers
on `.added`.

**Content parts** — `response.content_part.added` / `.done` carry
`{ item_id, output_index, content_index, part, sequence_number }`.

**Text deltas** — `response.output_text.delta`
`{ item_id, output_index, content_index, delta, logprobs?, sequence_number }`;
finalized by `response.output_text.done` `{ ..., text }`. Also
`response.output_text.annotation.added`, plus `response.refusal.delta`
/ `.done`.

**Function-call streaming** — sequence is:
1. `response.output_item.added` with `item = { type: "function_call",
   id, call_id, name, arguments: "" }`
2. zero or more `response.function_call_arguments.delta`
   `{ item_id, output_index, delta, sequence_number }` — append
   the raw JSON string into a buffer; do not parse partials.
3. `response.function_call_arguments.done`
   `{ item_id, output_index, name, arguments, sequence_number }` —
   `arguments` is the full JSON string; parse and dispatch.
4. `response.output_item.done` closes the item.

**Reasoning summary streaming** — `response.reasoning_summary_part.added`
/ `.done` and `response.reasoning_summary_text.delta` / `.done`
`{ item_id, output_index, summary_index, delta|text, sequence_number }`.

[streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events)

## 5. Reasoning / effort

```json
"reasoning": { "effort": "minimal", "summary": "auto" }
```

- **`effort` legal values**: `none`, `minimal`, `low`, `medium`,
  `high`, `xhigh` — **model-dependent**.
- **gpt-5.5**: supports `none, low, medium, high, xhigh` (default
  `medium`). `minimal` support across the 5.x line is
  **unconfirmed** for 5.5 specifically.
- **gpt-5.4 / gpt-5.4-mini / gpt-5.4-pro / gpt-5.3-codex /
  gpt-5.2**: the per-model "this effort is/isn't supported" matrix
  is **not nailed down on the public page**; the docs say
  "supported values are model-dependent." The note that
  **gpt-5.4-pro lacks `low`/`none` is unconfirmed** in current
  docs — verify via a probe call before relying on it.
- **`summary`**: `auto`, `concise`, `detailed`, `none`. `auto`
  picks the most detailed available for the model. Summaries
  arrive on `response.reasoning_summary_*` events; raw chain-of-
  thought is not exposed (encrypted reasoning items can be passed
  back via `include` for stateless multi-turn).

[reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)

## 6. Structured output

The Responses API uses **`text.format`**, not `response_format`
(that name is the Chat Completions form):

```json
"text": {
  "format": {
    "type": "json_schema",
    "name": "my_schema",
    "strict": true,
    "schema": { "type": "object",
                "properties": { "x": { "type": "string" } },
                "required": ["x"],
                "additionalProperties": false }
  }
}
```

`text.format.type` may also be `"text"` (default) or `"json_object"`
(legacy JSON mode).

`strict: true` enforces a subset of JSON Schema:
`additionalProperties: false` is required, all `properties` keys
must appear in `required`, and unsupported keywords are rejected.
Model output is then guaranteed to validate (or you get a
`refusal`).

Native support: GPT-4o (`gpt-4o-2024-08-06`+), `gpt-4o-mini`, and
the GPT-5 line. Per-model exclusions are **unconfirmed in current
docs**. Fall back to JSON mode (`{"type":"json_object"}`) plus
prompt-level schema instruction on unsupported models, per
providers.md R-WFWM-BKWX.

[structured outputs](https://platform.openai.com/docs/guides/structured-outputs)

## 7. Tool result round-trip

When the assistant emits an output item with
`type:"function_call"`, ikigai-cli runs the tool and returns the
result on the **next** request as an input item:

```json
{ "type": "function_call_output",
  "call_id": "call_abc123",
  "output": "<string — your serialized result>" }
```

`call_id` must echo the `call_id` from the assistant's
`function_call` item exactly. `output` is a string (stringify JSON
yourself). When sending under `store:false`, also re-include the
original `function_call` item in the input array so the model sees
its own prior tool call.

[function calling](https://developers.openai.com/api/docs/guides/function-calling)

## 8. Usage at completion

`response.completed.response.usage` shape:

```json
{
  "input_tokens": 1234,
  "input_tokens_details":  { "cached_tokens": 1024 },
  "output_tokens": 567,
  "output_tokens_details": { "reasoning_tokens": 256 },
  "total_tokens": 1801
}
```

- Read **only** from `response.completed`; do not aggregate deltas.
- **Prompt caching is automatic**: identical prefixes ≥1024 tokens
  are cached server-side; `cached_tokens` reports the hit and is
  billed at ~25% of normal input price. No client opt-in or cache-
  control header.
- `reasoning_tokens` are part of `output_tokens` and count against
  `max_output_tokens`.

[reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)

## 9. Conversation state — `store` and `previous_response_id`

- Default `store: true` → response persists 30 days; next call may
  pass `previous_response_id: "resp_..."` and only the new user
  item in `input`. Reasoning items are kept server-side and reused
  across turns.
- `store: false` → fully stateless. Send the entire transcript in
  `input` every call: original system/instructions, every prior
  `message`, every prior `function_call` + matching
  `function_call_output`. Reasoning items are dropped unless you
  opt into encrypted reasoning items via `include:
  ["reasoning.encrypted_content"]` and pass them back in the next
  `input`.
- ZDR organizations have `store:false` enforced.

For ralph-loops / ikigai-cli's full-transcript model,
**`store:false` is viable and recommended**: it keeps the wire
format symmetric and the loop owns history. Skip
`previous_response_id` entirely in this mode.

[conversation state](https://developers.openai.com/api/docs/guides/conversation-state)

## 10. Errors

**HTTP statuses**: `400` invalid request, `401` bad/missing key,
`403` country/permission, `404` no such model/response, `409`
conflict, `422` schema, `429` rate-limit or quota, `500` / `502`
/ `503` / `504` server. Retry on `429`, `5xx`, and connection
errors with exponential backoff + jitter.

**Error body**:

```json
{ "error": {
    "message": "...",
    "type":    "invalid_request_error|rate_limit_error|server_error|...",
    "param":   "tools[0].parameters" ,
    "code":    "invalid_value" } }
```

**Rate-limit headers (every response)**:

```
x-ratelimit-limit-requests
x-ratelimit-limit-tokens
x-ratelimit-remaining-requests
x-ratelimit-remaining-tokens
x-ratelimit-reset-requests
x-ratelimit-reset-tokens
retry-after            (on 429)
x-request-id           (always — log it)
```

`reset-*` values are durations like `"1s"`, `"6m0s"`. Values of
`-1`/`0` are an observed edge case; treat them as "unknown, back
off."

**Streaming errors**: mid-stream failures arrive as either
`event: response.failed` with
`data.response.error = { code, message }`, or an out-of-band
`event: error` with `{ code, message, param, sequence_number }`.
Both should abort the stream; only `error` is potentially
retryable without a new request.

[rate limits](https://developers.openai.com/api/docs/guides/rate-limits)

## Notes / unconfirmed

- platform.openai.com pages 403 to scrapers; developers.openai.com
  mirrors them and was the primary source.
- gpt-5.4-pro lacking `low`/`none` reasoning effort and the
  specific list of models that don't support strict structured
  output are **unconfirmed** against current official pages. Probe
  both before shipping policy logic.
- The `text.format` field name (vs `response_format` in Chat
  Completions) is a real, breaking difference. Easy bug source.
- Tool function shape is **flat** in the Responses API
  (`type/name/description/parameters/strict`), not the Chat
  Completions `{type:"function", function:{...}}` nesting.
