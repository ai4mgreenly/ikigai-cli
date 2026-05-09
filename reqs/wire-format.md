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

## Stdout: `user` events

- R-YLQR-3Z46: tool execution results appear as `user` events of
  shape
  `{"type":"user","message":{"role":"user","content":[<blocks>]}}`,
  emitted by ikigai-cli (not received from a real user) after each
  tool runs. ikigai-cli emits one user event per assistant turn
  that contained one or more tool_use blocks.

- R-Z6H1-M2PZ: each tool result is a content block of shape
  `{"type":"tool_result","tool_use_id":"<id>","is_error":<bool>,"content":<json-value>}`.
  The `tool_use_id` matches the `id` of the corresponding
  `tool_use` block from the assistant turn. `is_error` is
  `true` when the tool failed (per tools.md R-ZUM3-QUVT) and
  `false` otherwise. `content` may be a string, a structured JSON
  value, or null.

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

- R-13ZB-EZZK: the `result` event has shape
  `{"type":"result","structured_output":<json-value>,"is_error":<bool>}`.
  `structured_output` is the only field required for ralph-loops
  to make progress; `is_error` flags iteration-level failure.
  Optional fields (`num_turns`, `duration_ms`, `total_cost_usd`,
  `usage` with token counts) may be added in later versions for
  parity with `claude`'s verbose-mode output but are not part of
  the MVP contract.

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
  event MUST be answered by exactly one `tool_result` block in the
  next user event ikigai-cli emits, with matching `tool_use_id`,
  before the iteration's `result` event. ikigai-cli must not emit
  a `result` while there are unanswered tool calls pending.

## Out of contract

ralph-loops reads only the fields enumerated above. Anything else
in Claude Code's stream-json — `assistant.message.id` /
`stop_reason` / per-turn `usage`, block-level `cache_control`,
optional `result` fields beyond R-13ZB-EZZK — may be included or
omitted at ikigai-cli's discretion. Adding parity fields later is
not a breaking change.
