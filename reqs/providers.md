# Providers

This file specifies the per-provider surface ikigai-cli supports:
which models are recognized, what `--effort` values each model
accepts, and the provider-specific HTTP / SSE / tool-use behaviors
the implementation must map to and from.

The unifying principle: **ikigai-cli accepts each provider's native
vocabulary as written, validates per-model, and translates to and
from Claude Code's stream-json on the user-facing side.** No
universal effort scale; no cross-provider model aliasing; no
silently coerced values.

Model and effort data below reflects the state of each provider's
public API and first-party CLI as of early 2026. The data is
authoritative for the spec, but the implementation is expected to
keep this list current — adding a newly-released model by an
existing provider is a routine spec edit, not a re-architecture.

## Provider inference

- R-Y23Q-MNSU: the provider for a given `--model` value is
  determined by the bare API ID's prefix:
  - `claude-*` → Anthropic
  - `gpt-*` → OpenAI
  - `gemini-*` → Google
  Per-provider short aliases (Anthropic `opus`/`sonnet`/`haiku`
  with optional `[1m]` suffix; Google `pro`/`flash`/`flash-lite`/
  `auto`) resolve to bare IDs before inference. An unknown prefix
  is a fatal startup error (per OVERVIEW R-XBYO-1ZI1).

- R-YRPM-NUDF: model and effort validation is data-driven from a
  per-(provider, model) registry that lives in code as a const
  table, not loaded from disk. Adding a model means editing this
  table and shipping a new binary; this is consistent with the
  configless rule (OVERVIEW R-7IWS-GMJF).

- R-ZCFX-5XZ8: a `--model` value that parses to a known provider
  but is not in the registry is rejected at startup with an error
  listing the supported models for that provider. ikigai-cli does
  not attempt the request and let the provider 404.

- R-ZX67-O1L1: a `--effort` value that is not legal for the
  selected model (per the per-model effort vocabulary tables
  below) is rejected at startup with an error listing the legal
  values for that model. Two models on the same provider may have
  different legal effort sets.

## Anthropic

- R-0LK7-BGEX: ikigai-cli's Anthropic backend talks to the
  Anthropic Messages API directly over HTTPS, using Server-Sent
  Events (SSE) for streaming responses. It does not delegate to
  the real `claude` binary, even when the selected model is from
  Anthropic.

- R-18QA-L3I4: authentication uses the `ANTHROPIC_API_KEY` env
  var as a bearer credential per Anthropic's documented header
  format. No OAuth / Bedrock / Vertex routing in v1 — first-party
  Anthropic API only.

- R-1TGL-373X: cache-token usage statistics
  (`cache_read_input_tokens`, `cache_creation_input_tokens`) are
  populated on the result event's `usage` object from the
  Messages API response. Only Anthropic provides these; the other
  backends report zero (per wire-format.md R-2ANS-SYXV).

- R-2E6V-LAPQ: 1M-context support. When a model ID carries the
  `[1m]` suffix or the alias resolves to a 1M variant, the
  request is sent in 1M-context mode per Anthropic's current API
  conventions for that model. The exact mechanism (header vs
  parameter) follows whatever Anthropic documents at
  implementation time.

- R-31CY-UXSX: Anthropic backend in MVP supports a single model:
  `claude-haiku-4-5` (alias `haiku`). Haiku 4.5 takes no `--effort`
  argument; if supplied, ikigai-cli rejects it with an error
  listing the supported value (none). This deliberately avoids
  effort-validation work in MVP while exercising every other part
  of the agent loop. Opus 4.7, Sonnet 4.6, their `[1m]` variants,
  and the legacy 4.x models will be added in a later version once
  Haiku is round-tripping clean; the model registry per
  R-YRPM-NUDF must be shaped so that adding them is a data edit
  not an architecture change.

- R-4AH9-0G8M: tool-use round-trip is direct: the Messages API's
  `tool_use` and `tool_result` content blocks are isomorphic to
  Claude Code's stream-json blocks. The Anthropic backend's tool
  translation layer is essentially a passthrough with field-name
  normalization.

- R-4WFF-WBL4: Anthropic's `thinking` content blocks (when
  emitted by the model in adaptive-thinking mode) are forwarded
  to stdout as `thinking` blocks per wire-format.md R-SA9P-R1H4.

## OpenAI — DESIGN CONTEXT, NOT MVP

**The OpenAI section below is retained to inform the provider
abstraction's shape per OVERVIEW R-S04B-QD3D, but is NOT built in
MVP.** The build agent should not implement an OpenAI backend in
v1; the requirements here document what a v2 OpenAI backend must
satisfy and what differences from Anthropic the abstraction must
admit.



- R-5H5Q-EF6X: ikigai-cli's OpenAI backend talks to the OpenAI
  Responses API over HTTPS with SSE for streaming. The Chat
  Completions API is not used; codex-cli has deprecated it for
  agentic workflows and the Responses API is the documented
  forward path.

- R-64BT-O2A4: authentication uses the `OPENAI_API_KEY` env var
  as a bearer credential per OpenAI's documented header format.
  No Azure / organization-routing / project-key surface in v1.

- R-6P24-65VX: OpenAI backend supports the following models in
  v1, with the listed reasoning-effort vocabulary:

  | Model ID | Effort values | Notes |
  |---|---|---|
  | `gpt-5.5` | `none`, `low`, `medium`, `high`, `xhigh` | Default frontier; codex-cli default |
  | `gpt-5.4` | `none`, `low`, `medium`, `high`, `xhigh` | Mainline frontier |
  | `gpt-5.4-pro` | `medium`, `high`, `xhigh` | No low/none levels |
  | `gpt-5.4-mini` | `none`, `low`, `medium`, `high`, `xhigh` | Cost/latency tier |
  | `gpt-5.3-codex` | `low`, `medium`, `high`, `xhigh` | Coding specialist |
  | `gpt-5.2` | `none`, `low`, `medium`, `high`, `xhigh` | Previous frontier; still GA |

  No short aliases — codex-cli does not define any, and ikigai-cli
  follows suit (per OVERVIEW R-XBYO-1ZI1).

- R-79SE-O9HQ: `gpt-5.5-pro` is intentionally omitted from v1
  despite being a current frontier model. Its multi-minute
  request times and Responses-API-only nature are out of scope
  for the bounded ralph-loops iteration model. Add later if a use
  case emerges.

- R-7VQL-K4U8: tool-use translation. OpenAI's Responses API
  represents tool calls as `function_call` items in the streamed
  output and tool results as `function_call_output` items in the
  next request's input array. The OpenAI backend translates
  these to/from Claude Code's `tool_use` / `tool_result` blocks
  on the wire. Tool input arguments arrive as JSON-encoded
  strings on OpenAI's side; ikigai-cli decodes them into the
  JSON values that go into stream-json `tool_use.input`.

- R-8GGW-28G1: reasoning output. OpenAI emits `reasoning` items
  in the Responses API stream when reasoning models are used. By
  default ikigai-cli does NOT forward these as `thinking` blocks
  on stdout — the stream is internal to the model and not
  intended for end-user exposure. (This matches codex-cli's
  default behavior: reasoning summaries are surfaced in verbose
  mode only.)

- R-92F2-Y3SJ: structured-output enforcement for the iteration's
  final `result.structured_output` uses OpenAI's native
  Responses-API structured-output feature (response format with
  JSON schema) when supported by the selected model. For models
  that lack native structured outputs (e.g. `gpt-5.4-pro` per
  current API docs), the OpenAI backend falls back to prompt-
  level instruction plus local validation per R-WFWM-BKWX.

## Google Gemini — DESIGN CONTEXT, NOT MVP

**The Google Gemini section below is retained to inform the
provider abstraction's shape per OVERVIEW R-S04B-QD3D, but is NOT
built in MVP.** The build agent should not implement a Google
backend in v1; the requirements here document what a v2 Google
backend must satisfy and what differences from Anthropic the
abstraction must admit.

- R-9N5D-G7EC: ikigai-cli's Google backend talks to the
  Generative Language API (`generativelanguage.googleapis.com`)
  with the `streamGenerateContent` endpoint for streaming. Vertex
  AI routing is not supported in v1 — first-party Generative
  Language API only.

- R-AABG-PUHJ: authentication uses the `GOOGLE_API_KEY` env var
  as the API key per the Generative Language API's documented
  query-parameter or header format. No OAuth / service-account /
  ADC flows in v1.

- R-AXHJ-ZHKQ: Google backend supports the following models in
  v1, with the listed thinking vocabulary:

  | Model ID (bare) | Aliases | Effort values | Notes |
  |---|---|---|---|
  | `gemini-3.1-pro-preview` | `pro`, `auto` | `low`, `medium`, `high` | `thinkingLevel` keyword |
  | `gemini-3-pro-preview` | (alias `pro` in older configs) | `low`, `medium`, `high` | Superseded by 3.1 but still callable |
  | `gemini-3-flash-preview` | `flash` | `minimal`, `low`, `medium`, `high` | `thinkingLevel` keyword |
  | `gemini-3.1-flash-lite-preview` | `flash-lite` | `minimal`, `low`, `medium`, `high` | `thinkingLevel` keyword |
  | `gemini-2.5-pro` | (no alias mapped by default) | integer 128–32768 | `thinkingBudget` integer; cannot disable |
  | `gemini-2.5-flash` | (none) | integer 0–24576 | 0 disables; -1 dynamic |
  | `gemini-2.5-flash-lite` | (none) | integer 512–24576 | 0 disables; -1 dynamic |

  For 2.5 models, `--effort` accepts integer strings (parsed as
  `thinkingBudget`) or the literal `dynamic` (mapped to -1) or
  `off` (mapped to 0 where supported). For 3.x models, `--effort`
  accepts only the keyword strings.

- R-BKNN-94NX: 2.x and 3.x effort grammars are intentionally
  different. ikigai-cli does not normalize them — the user
  supplies what the chosen model expects, and validation rejects
  mismatches. This is the native-vocabulary rule (OVERVIEW
  R-XXWU-XUUJ) applied within a single provider.

- R-C7TQ-IRR4: tool-use translation. Gemini represents tool calls
  as `functionCall` parts within `candidates[].content.parts[]`
  and tool results as `functionResponse` parts in the next
  request's content. The Google backend translates these to/from
  Claude Code's `tool_use` / `tool_result` blocks. Function-call
  arguments arrive as already-decoded JSON objects on Gemini's
  side (unlike OpenAI's encoded-string form), so the translation
  is mostly structural.

- R-CTRX-EN3M: thinking output. Gemini does not stream thinking
  text to the client — only the final response — so there are no
  `thinking` blocks for ikigai-cli to forward from this provider.
  The `result.usage` object may include thinking-token counts;
  these are reported but not separately surfaced as content
  blocks.

- R-DFQ4-AIG4: structured-output enforcement uses Gemini's
  `responseSchema` feature on the request. All v1 Gemini models
  support it.

## Cross-cutting provider behavior

- R-ROBI-V64M: provider-side thinking / reasoning state must be
  preserved across all in-iteration round-trips with the same
  provider. When a multi-turn iteration uses tools, every request
  to the provider after the first carries the prior assistant
  turn's thinking/reasoning blocks intact in the conversation
  history.
  - **MVP — Anthropic correctness**: Haiku 4.5 supports adaptive
    thinking; signed thinking blocks paired with `tool_use` must
    be preserved or the API 400-rejects subsequent requests
    carrying the matching `tool_result`. Non-negotiable for any
    Anthropic iteration that uses tools.
  - **v2 — OpenAI quality**: under `store: false`, the backend
    must request `include: ["reasoning.encrypted_content"]` and
    round-trip `reasoning` items in subsequent `input` arrays.
  - **v2 — Gemini quality**: `thoughtSignature` on `thought` parts
    must be echoed back in subsequent contents.
  The abstraction must accommodate per-provider thinking-state
  preservation as a first-class concept, not an Anthropic-only
  hack bolted on later.

- R-WFWM-BKWX: delivering schema-conforming
  `result.structured_output` is ikigai-cli's responsibility for
  every supported model, not the provider's. Native structured-
  output features (OpenAI response_format JSON schema, Gemini
  responseSchema, Anthropic tool-call coercion patterns) are used
  as optimizations when available. When the selected model does
  not natively support schema-constrained output, the backend
  must fall back to prompt-level instruction plus local
  validation against the supplied `--json-schema`, retrying the
  model up to a bounded number of times before surfacing an
  iteration error. A model is not "supported" if ikigai-cli
  cannot guarantee structured output for it.

- R-E2W7-K5JB: provider HTTP/SSE errors, rate-limit responses,
  and connection timeouts terminate the iteration with a `result`
  event carrying `is_error: true`. ikigai-cli does not retry at
  the provider layer in MVP — ralph-loops owns retry policy at
  the loop layer. Raw HTTP status codes and response bodies must
  not leak onto stdout; the iteration-error result event is the
  only externally-visible failure surface.

- R-G0EH-D2SW: the provider abstraction layer's interface is the
  set of operations needed by the agent loop:
  - issue a streaming generation request given (model, effort,
    messages, tools, response-schema)
  - stream back normalized events (assistant text deltas,
    tool_use blocks, thinking blocks where applicable, usage
    totals, completion signal)
  - report errors as typed values, not raw HTTP errors
  Each backend implements this interface independently. The
  agent loop and the wire-format codec do not import provider-
  specific packages.
