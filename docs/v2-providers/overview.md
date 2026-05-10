# Google Gemini — Design Context (Not v1)

This document captures the Google Gemini provider design context
that informs ikigai-cli's provider abstraction shape. **It is not
part of the v1 spec.** The build agent does not implement the
Gemini backend in v1; this material exists so the abstraction
defined in `reqs/providers.md` can admit Gemini as an additive
package later.

When Gemini work begins, the operator should mint fresh
requirement IDs and place implementable claims under `reqs/`. The
text below preserves the original requirement IDs purely as
anchors for that future migration — they are **not** active
requirements while this file lives outside `reqs/`.

The Gemini wire-level reference lives at `google.md`. For the
Anthropic and OpenAI v1 implementation references, see
`reqs/providers/anthropic.md` and `reqs/providers/openai.md`.

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
