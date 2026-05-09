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
  is a fatal startup error (per R-XBYO-1ZI1).

- R-YRPM-NUDF: model and effort validation is data-driven from a
  per-(provider, model) registry that lives in code as a const
  table, not loaded from disk. Adding a model means editing this
  table and shipping a new binary; this is consistent with the
  configless rule (R-7IWS-GMJF).

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
  backends report zero.

- R-2E6V-LAPQ: 1M-context support. When a model ID carries the
  `[1m]` suffix or the alias resolves to a 1M variant, the
  request is sent in 1M-context mode per Anthropic's current API
  conventions for that model. The exact mechanism (header vs
  parameter) follows whatever Anthropic documents at
  implementation time.

- R-MPR7-P0A4: Anthropic backend in MVP supports two models:
  `claude-haiku-4-5` (alias `haiku`) and `claude-sonnet-4-6`
  (alias `sonnet`). Per-model effort vocabularies:
  - `claude-haiku-4-5` accepts no `--effort`; if supplied,
    ikigai-cli rejects it at startup per R-ZX67-O1L1 with an
    error listing the supported value (none). This preserves
    the original MVP narrowing for Haiku so the loop can run
    end-to-end without effort-validation work on at least one
    model.
  - `claude-sonnet-4-6` accepts `--effort` values
    `low | medium | high | xhigh | max`, matching Anthropic's
    adaptive-thinking effort vocabulary documented at
    `providers/anthropic.md` §5. Any other value is rejected at
    startup per R-ZX67-O1L1. The request is sent in adaptive-
    thinking mode (`thinking: {"type":"adaptive"}` paired with
    the supplied effort value) per `providers/anthropic.md`.
  Opus 4.7, the `[1m]` variants of either model, and the legacy
  4.x models remain deferred to a later version. The model
  registry per R-YRPM-NUDF must be shaped so adding them is a
  data edit, not an architecture change.

- R-4AH9-0G8M: tool-use round-trip is direct: the Messages API's
  `tool_use` and `tool_result` content blocks are isomorphic to
  Claude Code's stream-json blocks. The Anthropic backend's tool
  translation layer is essentially a passthrough with field-name
  normalization.

- R-4WFF-WBL4: Anthropic's `thinking` content blocks (when
  emitted by the model in adaptive-thinking mode) are forwarded
  to stdout as `thinking` blocks per wire-format.md R-SA9P-R1H4.

## OpenAI and Google — moved out of MVP spec

The v2 OpenAI and Google Gemini provider design context (which
informed the abstraction shape at R-S04B-QD3D) lives
under `docs/v2-providers/`:

- `docs/v2-providers/overview.md` — high-level v2 design notes
- `docs/v2-providers/openai.md` — OpenAI Responses API reference
- `docs/v2-providers/google.md` — Google Generative Language API reference

These are not requirements the MVP build agent must satisfy.
When v2 work begins, mint fresh requirement IDs and place
implementable claims back under `reqs/`. The cross-cutting
section below still mentions OpenAI and Gemini behaviors as
forward-looking obligations on the abstraction itself; those
references stay because they constrain the v1 abstraction's
shape, not because v1 implements them.

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
