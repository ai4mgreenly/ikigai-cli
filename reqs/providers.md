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

- R-ZZLK-I9CK: the model registry per R-YRPM-NUDF also carries
  per-model pricing data: at minimum, dollar rates per million
  input tokens and per million output tokens, plus separate
  rates for any cache-read or cache-creation discount the
  provider publishes for that model. This pricing data is the
  sole source ikigai-cli uses to compute `result.total_cost_usd`
  and `result.modelUsage[<model>].costUSD` per
  wire-format.md R-Y5QZ-UNB2 and providers.md R-YSX3-4AE9. Prices
  drift; updating a published rate is a routine spec edit and a
  registry-data edit, not an architecture change. A model whose
  pricing is unknown cannot ship: every entry in the registry
  must declare every rate it bills on, otherwise the cost
  totals would be silently wrong.

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

## OpenAI

- R-WWTI-LSSO: ikigai-cli's OpenAI backend talks to the OpenAI
  Responses API directly over HTTPS at
  `POST https://api.openai.com/v1/responses`, using Server-Sent
  Events (SSE) for streaming responses. The Chat Completions
  endpoint (`/v1/chat/completions`) is not used and not supported
  in v1. This is load-bearing: the Responses API is the only
  surface that exposes signed reasoning round-trip
  (`reasoning.encrypted_content`) needed to satisfy R-ROBI-V64M
  for OpenAI.

- R-0W9B-7E8I: authentication uses the `OPENAI_API_KEY` env var
  as a bearer credential — `Authorization: Bearer <key>`. No
  `OpenAI-Organization` header, no `OpenAI-Project` header, no
  Azure OpenAI routing in v1 — first-party OpenAI API only. A
  future Azure / org-routing surface is a v-bump decision, not a
  flag-driven extension.

- R-1GZL-PHUB: OpenAI backend in MVP supports exactly one model:
  `gpt-5.5`. Legal `--effort` values for `gpt-5.5` are
  `none | low | medium | high | xhigh`. Any other value
  (including `minimal`, which is legal on some other 5.x models
  but not on gpt-5.5) is rejected at startup per R-ZX67-O1L1
  with an error listing the five legal values. Other 5.x family
  members (`gpt-5.5-pro`, `gpt-5.5-mini`, etc.) and earlier
  generations are deferred; the model registry per R-YRPM-NUDF
  must be shaped so adding them is a data edit, not an
  architecture change.

- R-22XS-LD6T: when `--effort` is omitted on `--model=gpt-5.5`,
  ikigai-cli sends `reasoning.effort: "medium"` explicitly in
  the request body. The default is pinned by the model registry,
  not deferred to OpenAI's server-side default; this keeps
  `--raw` traces under R-92NN-7DNI faithful to what was actually
  requested and insulates ikigai-cli's behavior from drift in
  OpenAI's defaults.

- R-2RBS-8S0P: tool-use translation. OpenAI's Responses API
  represents tool calls as `function_call` items in the streamed
  output and tool results as `function_call_output` items in the
  next request's `input` array. The OpenAI backend translates
  these to and from Claude Code's `tool_use` / `tool_result`
  blocks on the wire per R-ZRRF-LGW1. Tool input arguments
  arrive as JSON-encoded strings on OpenAI's side; ikigai-cli
  decodes them into the JSON values that go into stream-json
  `tool_use.input`, and re-encodes when emitting tool calls back
  to OpenAI on subsequent turns.

- R-ZEVA-05QR: usage mapping. The OpenAI Responses API reports
  iteration-level usage on `response.completed.usage` (cumulative
  across all turns made within one request, with reasoning
  tokens already folded into `output_tokens`). The OpenAI backend
  maps it onto the standard shape required by R-YSX3-4AE9:
  - `usage.input_tokens` ← OpenAI `usage.input_tokens`
  - `usage.output_tokens` ← OpenAI `usage.output_tokens`
    (reasoning tokens are already included; do not add them
    separately or double-count)
  - `usage.cache_read_input_tokens` ← OpenAI
    `usage.input_tokens_details.cached_tokens`
  - `usage.cache_creation_input_tokens` ← `0` (OpenAI has no
    cache-creation concept; the field is reported as zero per
    R-YSX3-4AE9)
  Cost contribution toward `total_cost_usd` and the per-model
  `modelUsage[<model>].costUSD` is computed from these counts
  using the OpenAI model's pricing entry in the registry per
  R-ZZLK-I9CK; OpenAI does not expose a billing dollar amount
  on the response, and ikigai-cli does not query a billing API.

- R-3V3G-PYML: tool-definition adaptation. The OpenAI backend
  rewrites each tool's neutral input schema (per tools.md
  R-YNXM-CVXI) into the strict-mode shape OpenAI's Responses API
  enforces when `strict: true` is set on a function tool: every
  object level declares `additionalProperties: false`, every
  property declared in `properties` is listed in `required`, and
  fields that are optional in the neutral schema are expressed as
  nullable union types. The neutral schema in the tool definition
  is unchanged; adaptation is per-request, on the wire. This is
  the OpenAI-specific instance of R-3959-U3A3 and is what makes
  Claude-Code-shaped tools usable on the Responses API without
  forcing every tool author to encode OpenAI's strict-mode rules.

- R-3D9Z-4ND7: stateless reasoning round-trip. The OpenAI
  backend always sends `store: false` and (whenever a reasoning
  model is in use) `include: ["reasoning.encrypted_content"]`.
  `reasoning` items returned by the model are appended to the
  conversation history unchanged — same `id`, same
  `encrypted_content` — and replayed in the `input` array of
  every subsequent request in the same iteration. This is
  OpenAI's equivalent of Anthropic's signed-thinking
  preservation and satisfies R-ROBI-V64M for the OpenAI
  backend. Server-side state (`previous_response_id`,
  `store: true`) is not used in v1.

- R-3Z86-0IPP: structured-output enforcement uses OpenAI's
  native Responses-API feature
  (`text.format: {type: "json_schema", strict: true, schema:
  ...}`). The schema supplied via `--json-schema` (per
  R-JNEB-EVLU) is forwarded verbatim into `text.format.schema`
  with `strict: true`. There is no prompt-level fallback path
  in v1: gpt-5.5 supports native strict schema enforcement, so
  R-WFWM-BKWX's prompt-and-validate fallback is not exercised
  on the OpenAI backend. If the supplied schema fails OpenAI's
  strict-mode validation (e.g. missing `additionalProperties:
  false`, optional fields not declared as nullable unions),
  the resulting 400 is surfaced as an iteration error per
  R-E2W7-K5JB without translation — repairing the schema is
  the operator's responsibility.

- R-4JYG-IMBI: reasoning summaries are not surfaced on stdout.
  The OpenAI backend does not request `reasoning.summary` and
  does not forward `response.reasoning_summary_*` events as
  Claude `thinking` blocks. The `encrypted_content` round-trip
  per R-3D9Z-4ND7 is internal to the conversation history; the
  human-readable summary stream is not part of the v1 wire
  surface. This keeps the OpenAI backend's stream-json output
  uniform with the Anthropic backend's output for non-thinking
  models, and avoids leaking reasoning text that the Responses
  API itself does not consider end-user-visible.

- R-574J-S9EP: the OpenAI backend maps Responses-API HTTP and
  SSE error shapes into the `provider.ErrorKind` taxonomy:
  `authentication_error` → `ErrAuth`; `invalid_request_error`
  → `ErrInvalidRequest`; `rate_limit_error` → `ErrRateLimit`;
  read/connect timeouts → `ErrTimeout`;
  `server_error` / `overloaded_error` and other 5xx →
  `ErrServer`; anything else → `ErrUnknown`. Raw HTTP status
  codes and response bodies do not reach stdout, per
  R-E2W7-K5JB.

- R-5RUU-AD0I: the system prompt under R-8PF6-I8FP is delivered
  to the Responses API via the top-level `instructions` field,
  not as a `developer`- or `system`-role item inside `input`.
  This keeps the framing prompt out of conversation history and
  matches the Responses API's documented precedence (an
  `instructions` value outranks any `developer`-role item, and a
  `developer`-role item outranks any `system`-role item).

The implementation-grade wire reference for the OpenAI backend
lives at `providers/openai.md`.

## Google Gemini — design context

Gemini is deferred to a later version; the design context that
shaped the v1 abstraction lives under `docs/v2-providers/`:

- `docs/v2-providers/overview.md` — high-level v2 design notes
- `docs/v2-providers/google.md` — Google Generative Language API reference

These are not requirements the v1 build agent must satisfy.
When Gemini work begins, mint fresh requirement IDs and place
implementable claims back under `reqs/`. The cross-cutting
section below still mentions Gemini behaviors as forward-looking
obligations on the abstraction itself; those references stay
because they constrain the v1 abstraction's shape, not because
v1 implements Gemini.

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
  - **MVP — OpenAI correctness**: under `store: false`, the
    backend must request `include: ["reasoning.encrypted_content"]`
    and round-trip `reasoning` items in subsequent `input`
    arrays. See R-3D9Z-4ND7. Non-negotiable for any OpenAI
    iteration that uses tools or reasoning.
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

- R-YSX3-4AE9: each backend populates the `result` event's
  `usage`, `total_cost_usd`, `num_turns`, `duration_ms`, and
  `modelUsage` fields per wire-format.md R-Y5QZ-UNB2. The standard
  `usage` shape is Claude Code's; backends map their provider's
  native usage onto it. Fields the underlying provider does not
  expose (e.g. cache-creation token counts on a backend with no
  cache-creation concept) are reported as zero, not omitted.
  `total_cost_usd` and per-model `costUSD` are computed from the
  per-model pricing data in the model registry (R-ZZLK-I9CK)
  applied to the iteration's token totals; they are not taken
  from any provider-side billing field. Per-provider mapping
  details live in the provider sections (R-1TGL-373X for
  Anthropic cache fields, R-ZEVA-05QR for OpenAI).

- R-3959-U3A3: each provider is responsible for adapting the
  neutral tool input schemas (Claude-Code-shaped, per tools.md
  R-YNXM-CVXI) into whatever shape its wire format requires
  before transmission. The neutral schema declared by the tool
  is the advertised contract; per-request adaptation to backend-
  specific constraints (additional-properties policies, required-
  property rules, optional-field encodings, naming conventions)
  is owned by the backend that needs them. This isolates per-
  provider quirks to the provider that introduces them, so tool
  authors and other backends are not forced to encode constraints
  that don't apply to them.

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
