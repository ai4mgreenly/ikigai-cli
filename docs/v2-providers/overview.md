# v2 Providers — Design Context (Not MVP)

This document captures the OpenAI and Google Gemini provider design
context that informs ikigai-cli's provider abstraction shape. **It
is not part of the MVP spec.** The build agent does not implement
these backends in v1; this material exists so the abstraction
defined in `reqs/providers.md` (Anthropic + cross-cutting) can
admit them as additive packages later.

When v2 work begins, the operator should mint fresh requirement
IDs and place implementable claims under `reqs/`. The text below
preserves the original requirement IDs purely as anchors for that
future migration — they are **not** active requirements while this
file lives outside `reqs/`.

Implementation references for each provider live alongside this
document:

- `openai.md` — OpenAI Responses API wire-level reference
- `google.md` — Google Generative Language API wire-level reference

For the Anthropic v1 implementation reference, see
`reqs/providers/anthropic.md`.

## OpenAI — design context

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

## Google Gemini — design context

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
