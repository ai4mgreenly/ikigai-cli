# Wire format

This file specifies the byte-level contract between ikigai-cli and
ralph-loops over stdin and stdout. The contract surface is **exactly
what ralph-loops actually parses** — no more, no less. Fields and
event types ralph-loops doesn't read are freedom for ikigai-cli;
fields it does read are obligations.

The format is Claude Code's `stream-json` (the format selected by
`--input-format stream-json --output-format stream-json` on the real
`claude` binary). ikigai-cli must speak it well enough that
ralph-loops cannot tell the difference for the events it cares
about.

## Transport

- R-RCS9-92FK: stdin and stdout are newline-delimited JSON. Each
  line is exactly one complete JSON object (an event). No
  pretty-printing, no leading/trailing whitespace inside the line,
  no embedded newlines within string values that would split a
  logical event across lines.

- R-RYQG-4XS2: encoding is UTF-8. Strings containing non-ASCII
  characters are emitted as valid UTF-8, not escaped to `\uXXXX`
  sequences (escaping is permitted but not required; both must
  parse identically on ralph-loops' side).

- R-SJGQ-N1DV: each output line is at most 16 MiB after encoding.
  This is ralph-loops' scanner buffer cap. Tool results that would
  exceed this limit must be truncated by the emitting tool (per
  tools.md), not allowed to overflow at the wire layer.

- R-T6MT-WOH2: stdout writes are line-flushed. ralph-loops reads
  events line-by-line as the model produces them; ikigai-cli must
  not buffer multiple events together waiting for the iteration to
  finish.

- R-TSL0-SJTK: every event object has a top-level `type` string
  that discriminates its shape. The set of types ikigai-cli emits
  in v1 is `assistant`, `user`, `result`. (`system` and
  `rate_limit_event` are part of Claude Code's stream-json
  surface but ralph-loops only consumes them in verbose mode and
  behaves correctly without them; they may be added in a later
  version.)

## Stdin: events ikigai-cli accepts

- R-UDBB-ANFD: ikigai-cli reads exactly one event type from stdin
  in v1: `user` events with shape
  `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<prompt>"}]}}`.
  This is what ralph-loops sends at iteration kickoff and on
  retry. Other event types on stdin are not part of the v1
  contract and may be rejected.

- R-V0HE-KAIK: stdin is closed by ralph-loops when the iteration
  ends or is cancelled. ikigai-cli treats EOF on stdin as a signal
  that no further user input will arrive but does not itself
  terminate on EOF — the iteration ends when ikigai-cli emits a
  `result` event, not when stdin closes.

## Stdout: `assistant` events

- R-VUYW-4K1X: every `assistant` event has shape
  `{"type":"assistant","message":{"role":"assistant","content":[<blocks>]}}`.
  The `content` array contains zero or more typed content blocks.
  ikigai-cli emits one assistant event per assistant turn from the
  underlying provider.

- R-WQOA-2LBZ: text output from the model appears as a content
  block of shape `{"type":"text","text":"<string>"}`. Multiple
  text blocks within a single assistant turn are permitted.

- R-XCMG-YGOH: tool invocations appear as content blocks of shape
  `{"type":"tool_use","id":"<unique-id>","name":"<tool-name>","input":<json-value>}`.
  The `id` is a string unique within the iteration that ralph-
  loops uses to correlate the resulting `tool_result`. The
  `input` is a JSON value (object, string, or null) representing
  the tool arguments.

- R-SA9P-R1H4: extended-thinking / reasoning output, when the
  underlying provider emits it, is forwarded to stdout as content
  blocks of shape `{"type":"thinking","thinking":"<string>"}`.
  ikigai-cli already preserves these blocks internally per
  providers.md R-ROBI-V64M (a correctness requirement on
  Anthropic, a quality requirement elsewhere); emitting them on
  stdout costs essentially nothing on top of that and matches the
  real `claude` binary's verbose-mode output. Ralph-loops only
  logs their length, but the parity is worth keeping. Providers
  that don't expose thinking text to the client (Gemini without
  `includeThoughts: true`) emit no thinking blocks; that's
  acceptable.

- R-FPG8-RKEP: `thinking` content blocks emitted on stdout MUST
  carry non-empty `thinking` text. Some providers raise thinking
  events whose only payload is provider-internal reasoning state
  needed to round-trip the next request — Google's
  `thoughtSignature` (providers.md R-P1V4-NTDY) and OpenAI's
  reasoning-item `encrypted_content` (providers.md R-3D9Z-4ND7).
  Those signature-only events are conversation-history mechanics,
  not human-readable thinking; surfacing them as
  `{"type":"thinking","thinking":""}` adds noise to the operator's
  transcript and contradicts the provider-level requirements that
  thinking summaries not be surfaced (R-QKQL-VHR7 for Google,
  R-4JYG-IMBI for OpenAI). They must be filtered before stdout
  while still being preserved in the provider conversation history
  so the next request can replay them.

## Stdout: `user` events

- R-EW6N-L2M1: tool execution results appear as `user` events of
  shape
  `{"type":"user","message":{"role":"user","content":[<blocks>]}}`,
  emitted by ikigai-cli (not received from a real user) after each
  tool runs. ikigai-cli emits exactly one `user` event per
  `tool_result` block: when an assistant turn contains N
  `tool_use` blocks, ikigai-cli emits N user events on stdout,
  each carrying a single `tool_result` content block in
  `message.content` (and its R-CZWA-5X35 sidecar where the tool
  has one). One event per result keeps the per-tool sidecar
  unambiguously paired with its block and matches the real
  `claude` binary's emission pattern; downstream renderers
  (notably ralph-loops') key off that pairing. Replaces and
  retires R-YLQR-3Z46, whose per-turn batching could not express
  multi-tool sidecars in the Claude Code wire shape.

- R-Z6H1-M2PZ: each tool result is a content block of shape
  `{"type":"tool_result","tool_use_id":"<id>","is_error":<bool>,"content":<json-value>}`.
  The `tool_use_id` matches the `id` of the corresponding
  `tool_use` block from the assistant turn. `is_error` is
  `true` when the tool failed (per tools.md R-ZUM3-QUVT) and
  `false` otherwise. `content` may be a string, a structured JSON
  value, or null.

- R-CZWA-5X35: a `user` event carrying a `tool_result` block MAY
  also carry a top-level `tool_use_result` field — a sidecar
  alongside `message`, not nested inside it — whose value is the
  tool-specific Claude Code CLI sidecar shape. Tools whose
  Claude Code counterpart emits a sidecar populate it (Bash:
  see tools.md R-DPI6-73NQ); tools whose Claude Code counterpart
  does not omit the field entirely. The sidecar exists so
  downstream renderers can present a tool_result faithfully —
  e.g. distinguishing stderr from stdout for Bash — without
  re-parsing the model-facing combined `content` string. This is
  Claude Code wire-format parity: the field is part of the real
  `claude` binary's stream-json output that ralph-loops already
  consumes for Claude-driven runs, so omitting it for ikigai-cli
  runs makes the same renderer behave differently depending on
  the engine.

- R-ZUV1-9HJV: ralph-loops echoes its own user input back as a
  `user` event with `{"type":"text","text":"..."}` content blocks
  when the real claude binary is run with `--replay-user-messages`.
  ikigai-cli must replay user input the same way: when it consumes
  a stdin user event, it emits an equivalent `user` event on
  stdout before the model's response begins.

## Stdout: `result` events (mandatory terminator)

- R-0I14-J4N2: every iteration ends with exactly one `result`
  event emitted on stdout. No assistant or user events follow the
  result event within the same iteration; ralph-loops uses the
  result event as the terminator and stops reading.

- R-Y5QZ-UNB2: the `result` event has the shape Claude Code's
  stream-json verbose-mode result event has. The fields ralph-loops
  reads to make progress and to report iteration accounting:
  - `type: "result"`
  - `structured_output: <json-value>` — required for ralph-loops
    to make progress
  - `is_error: <bool>` — iteration-level failure flag
  - `num_turns: <int>` — assistant turns within the iteration
  - `duration_ms: <int>` — wall-clock duration of the iteration
  - `total_cost_usd: <number>` — dollar cost of the iteration,
    summed across every assistant turn and every model used
  - `usage: { input_tokens, output_tokens,
    cache_read_input_tokens, cache_creation_input_tokens, ... }`
    — cumulative token counts for the iteration in Claude Code's
    shape. Provider-specific subfields (e.g. Anthropic's
    `cache_creation.ephemeral_5m_input_tokens` /
    `ephemeral_1h_input_tokens`) are populated when the backend
    exposes them; otherwise the field is absent or zero.
  - `modelUsage: { "<model-id>": { inputTokens, outputTokens,
    cacheReadInputTokens, cacheCreationInputTokens, costUSD,
    contextWindow, maxOutputTokens, ... } }` — per-model
    breakdown, keyed by the full model id used during the
    iteration.
  Per-backend mapping of the underlying provider's native usage
  fields onto this standard shape is governed by providers.md
  R-YSX3-4AE9 and the per-provider mapping requirements there.
  Replaces and retires R-13ZB-EZZK.

- R-ZRNK-LQRK: ikigai-cli does **not** emit `message.usage` on
  per-`assistant` events, even though Claude Code does. The
  per-message `usage` Claude Code emits during streaming is
  partial — output_tokens in particular reflects whatever has
  streamed so far, not the message's final count — so summing or
  reading it across events produces wrong totals. The `result`
  event's `usage` (per R-Y5QZ-UNB2) is the single authoritative
  source of token accounting for an iteration. This is a
  deliberate divergence from Claude Code's wire shape; the
  motivating principle (give consumers correct totals, not
  Claude-Code-flavored partial ones) outranks bit-for-bit parity
  in this one place.

- R-1OPL-X3LD: `structured_output` must be a JSON value that
  validates against the schema supplied via `--json-schema`. For
  ralph-loops this is always `{"status":"DONE"|"CONTINUE"}`. If
  ikigai-cli cannot coax the model into producing valid structured
  output within the iteration, it emits a `result` event with
  `is_error: true` and a structured_output value that fails
  validation; ralph-loops will retry up to its configured limit.

## Tool correlation and ordering

- R-5DMN-M3F2: tool_use and tool_result blocks are correlated by
  `id` / `tool_use_id`, not by ordering. Within a single user
  event, tool_result blocks may appear in any order relative to
  the originating tool_use blocks; ralph-loops looks them up by
  id.

- R-5ZKU-HYRK: every `tool_use` block emitted in an assistant
  event MUST be answered by exactly one `tool_result` block —
  carried in one of the `user` events ikigai-cli emits before the
  iteration's `result` event, with matching `tool_use_id`.
  Whether those answers arrive bundled in one user event or one
  per event is governed by R-EW6N-L2M1; the invariant here is
  the answered-exactly-once-before-result obligation. ikigai-cli
  must not emit a `result` while there are unanswered tool calls
  pending.

## Out of contract

ralph-loops reads only the fields enumerated above. Anything else
in Claude Code's stream-json — `assistant.message.id` /
`stop_reason`, block-level `cache_control`, optional `result`
fields beyond R-Y5QZ-UNB2 — may be included or omitted at
ikigai-cli's discretion. Adding parity fields later is not a
breaking change. Per-`assistant` `message.usage` is explicitly
*not* emitted (R-ZRNK-LQRK).
