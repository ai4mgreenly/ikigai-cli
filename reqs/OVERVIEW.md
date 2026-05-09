# ikigai-cli

A command-line agent binary that ralph-loops can spawn in place of
`claude`, transparently backed by Anthropic, OpenAI, or Google Gemini.
The single most important thing it must do is present Claude Code's
stream-json wire format on stdin/stdout regardless of which provider
answers the request — so ralph-loops needs zero changes to switch
backends.

## Goal

Let ralph-loops drive any supported provider with a single binary
swap, while ralph-loops continues to speak only Claude's protocol.
ikigai-cli is the translator: it accepts Claude-shaped input, runs
the conversation against a chosen provider's HTTP API, executes
locally-implemented tools as part of the agent loop, and emits
Claude-shaped events back out.

## Audience

ralph-loops, and ralph-loops alone in v1. Human users, other
orchestrators, IDE integrations, and the broader `claude` CLI surface
are explicitly not part of the target audience.

## MVP scope

- R-S04B-QD3D: v1 implements Anthropic only. OpenAI and Google
  Gemini are deliberately deferred to a later version. The
  provider abstraction, model registry shape, effort vocabulary
  (native pass-through), and tool-runtime / wire-format
  separation must nonetheless be designed assuming all three
  providers will eventually be supported — adding OpenAI or
  Gemini in a later version must not require re-architecting the
  agent loop, the wire-format codec, the tool runtime, or the
  provider interface. The reference docs at `providers/openai.md`
  and `providers/google.md` are kept in this directory as design
  context (so the abstraction is shaped against real provider
  differences), not as MVP build targets.

## Configuration model

- R-7IWS-GMJF: ikigai-cli is configless. It runs from built-in
  defaults; the only way to change behavior is CLI flags. There is
  no config file, no `~/.ikigai/`, no `IKIGAI_*` environment-
  variable knobs, no `--config` flag, and no autoload of any
  settings file. The only environment variables ikigai-cli reads
  are the provider API keys defined in R-YL2Y-7HXQ. Any other
  environment variable ralph-loops or the operator may set is
  ignored (accepted silently when known to come from `claude`'s
  surface, e.g. `CLAUDE_CONFIG_DIR`; otherwise simply not read).

## Hard requirements

- R-YARD-835I: ralph-loops can invoke ikigai-cli with the exact same
  flag set, environment variables, stdin protocol, and signal
  handling it uses for `claude`, and complete an iteration end-to-end
  without any change to ralph-loops itself. This is the load-bearing
  drop-in invariant; everything else exists to make it true.

- R-UXDS-W9UQ: every line ikigai-cli writes to stdout is a valid
  Claude Code stream-json event (`assistant`, `user`, `result`,
  `system`, `rate_limit_event`, or a forward-compatible variant),
  regardless of which provider produced the underlying response. The
  shape on the wire must be indistinguishable from `claude` for the
  events ralph-loops actually consumes.

- R-VJBZ-S578: a single iteration terminates with exactly one
  `result` event whose `structured_output` satisfies the JSON schema
  passed via `--json-schema`. For ralph-loops this is always
  `{"status":"DONE"|"CONTINUE"}`, but the binary must honor whatever
  schema is provided.

- R-A351-VO9A: ikigai-cli processes stdin event-by-event and MUST
  NOT wait for stdin EOF before dispatching the agent loop. As
  soon as one complete `user` stream-json event has been parsed
  off stdin, ikigai-cli initiates the provider round-trip with
  that user message as the prompt; subsequent stream-json events
  (none, in ralph-loops' current usage) are handled in the same
  event-driven manner. The iteration terminates by writing the
  single `result` event to stdout per R-VJBZ-S578 — at no point
  is stdin EOF a precondition for emitting that result. ralph-
  loops keeps stdin open against `claude` today and `claude`
  responds without waiting for EOF; the drop-in invariant
  (R-YARD-835I) requires ikigai-cli to behave identically.
  Buffering stdin to EOF before invoking the provider deadlocks
  the iteration: ralph-loops will not close its end of the pipe
  until it has read ikigai-cli's result, and ikigai-cli will not
  emit that result until its read of stdin returns EOF. This
  failure mode is silent (no HTTP traffic, no `[llm>]` trace
  entry under R-92NN-7DNI) and is exactly the symptom this
  requirement exists to forbid.

- R-JNEB-EVLU: the value of `--json-schema` is the JSON Schema
  document itself, passed inline as a literal JSON string on the
  command line — not a path to a file containing the schema.
  ralph-loops constructs the flag as
  `--json-schema '{"type":"object", ...}'` (see ralph-loops'
  `internal/agent/engine.go`); ikigai-cli must parse the value
  directly as JSON. Treating the value as a filesystem path is a
  bug that surfaces as an `open: no such file or directory` error
  on every invocation. File-path input is not supported in v1; if
  it is added later it must be opt-in via a sentinel prefix (e.g.
  `@path/to/schema.json`) so the bare-value form continues to mean
  "inline literal".

- R-W5A6-O0JQ: ikigai-cli speaks to provider HTTP APIs directly. No
  vendor SDK (Anthropic, OpenAI, Google) may be linked. General-
  purpose libraries — HTTP client, SSE parser, JSON, CLI framework,
  filesystem helpers — are permitted.

- R-92NN-7DNI: ikigai-cli accepts a `--raw` boolean flag that
  defaults to `false`. When `--raw` is `true`, ikigai-cli emits
  a debug trace to **stdout** that MUST cover all three of the
  following boundaries — partial coverage of any one boundary
  does not satisfy this requirement:
  1. **Every message read from stdin.** As soon as ikigai-cli
     parses one stream-json event off its stdin, it writes a
     trace entry containing that event's complete raw bytes.
     Prefix: `[stdin>] `. This is the visible proof that
     ikigai-cli has consumed the operator's input.
  2. **Every message sent to the provider's LLM.** As soon as
     ikigai-cli sends an HTTP request to the provider's
     model-completion endpoint, it writes a trace entry
     containing the complete request body (the JSON sent to
     the LLM), the request URL, and the request method.
     Prefix: `[llm>] `. One trace entry per HTTP request — if
     a single iteration makes multiple round-trips (because of
     tool-use turns), every round-trip's outbound body is
     logged.
  3. **Every message received from the provider's LLM.** As
     bytes arrive from the provider over HTTP / SSE,
     ikigai-cli writes a trace entry per SSE event (or, for a
     non-streaming response, one entry for the complete body)
     containing the raw payload received and the HTTP status
     code that opened the response. Prefix: `[<llm] `. A hung
     iteration must surface as a `[llm>] ` with no following
     `[<llm] ` (or with a partial sequence stopping at a
     specific event), so the operator can see exactly where
     the LLM round-trip stalled.
  Trace formatting rules (apply to all three boundaries):
  - Each trace entry begins on a fresh line — ikigai-cli
    writes a leading newline before its prefix if the previous
    byte on stdout was not already `\n`. This prevents trace
    entries from being mashed onto the same visual line as
    upstream output (e.g. ralph-loops' `--raw` echo).
  - Each entry carries an RFC 3339 timestamp with subsecond
    precision immediately after its prefix.
  - Multi-line payloads are written verbatim; each next trace
    entry's leading newline keeps boundaries unambiguous.
  When `--raw` is `true` the stdout-is-only-stream-json
  invariant of R-UXDS-W9UQ is intentionally relaxed: trace
  entries are interleaved with stream-json events on the same
  stream. ralph-loops only passes `--raw` to ikigai-cli when
  the operator has run ralph with its own `--raw` flag, in
  which case ralph dumps the engine's stdout verbatim rather
  than parsing it. When `--raw` is `false`, R-UXDS-W9UQ holds
  unchanged.
  Provider API key values MUST NEVER appear in trace output.
  Redaction applies to every header field, query parameter, or
  body field whose value matches a configured API key — not
  only to the `Authorization` header. (Note: the request and
  response bodies covered by `[llm>] ` and `[<llm] ` should
  not contain the API key in normal operation; redaction
  exists as a defense in depth.)
  This is an MVP debug aid; the flag and its behavior are
  expected to be replaced by structured tracing or removed
  once the loop is proven end-to-end. The flag belongs to
  ikigai-cli itself, not to the ralph drop-in flag set pinned
  by R-6TC0-ZSKM / R-Z4YN-KG36.

- R-2247-BPXI: startup errors — missing or unreadable provider
  credentials (R-YL2Y-7HXQ), unknown `--model` prefix
  (R-XBYO-1ZI1), unknown model in the registry (R-ZCFX-5XZ8),
  illegal `--effort` for the selected model (R-ZX67-O1L1), and
  unrecognized CLI flags — are reported by writing a single
  human-readable message to stderr that names the specific
  problem (for missing credentials, the env var that was unset
  and the provider it belongs to), exiting with a non-zero
  status, and writing nothing to stdout. No `result` event, no
  `system` event, no partial stream-json on stdout. The
  orchestrator's "stream ended without result" path is the
  intended consumer of this failure mode; in-iteration failures
  continue to follow R-E2W7-K5JB. The usage block printed by the
  Go flag parser on unknown-flag rejection counts as the human-
  readable stderr message and satisfies this requirement.

## Provider model

- R-WQ0H-645J: v1 supports three providers: Anthropic, OpenAI, and
  Google Gemini. Adding a fourth provider must not require changes
  to the wire-format translation layer or to existing tool
  implementations.

- R-XBYO-1ZI1: the `--model` flag always accepts the bare API model
  ID used by that provider's HTTP API (e.g. `claude-opus-4-7`,
  `gpt-5.4`, `gemini-3-pro-preview`). Per-provider short aliases
  may also be accepted as sugar where the upstream first-party CLI
  defines them (Anthropic: `opus`/`sonnet`/`haiku` and bracketed
  variants like `opus[1m]`; Google: `pro`/`flash`/`flash-lite`/
  `auto`; OpenAI: none). The provider is inferred from the bare ID
  prefix (`claude-*`, `gpt-*`, `gemini-*`); no separate `--provider`
  flag exists. Unknown prefixes are a fatal startup error.

- R-XXWU-XUUJ: the `--effort` flag is passed through in each
  provider's and model's native vocabulary, not normalized to a
  universal scale. ikigai-cli validates that the supplied value is
  legal for the chosen model and rejects it loudly otherwise. Two
  models from the same provider may accept different effort
  vocabularies.

- R-YL2Y-7HXQ: provider credentials are read from environment
  variables only — `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
  `GOOGLE_API_KEY`. There is no credentials-file fallback, no
  keychain integration, no alternative variable names. A future
  fourth provider must follow the same `<PROVIDER>_API_KEY`
  pattern. Missing credentials for the selected provider are a
  fatal startup error.

## Tool runtime

- R-Z5T8-PLJJ: ikigai-cli implements Claude Code's built-in tools
  itself and exposes them to the underlying model via that
  provider's native tool-use mechanism. The Anthropic backend is not
  a passthrough to the real `claude` binary — it uses the Messages
  API and runs tools locally, same as the other backends.

- R-YFCR-J9IL: the `--tools` flag accepts a comma-separated list
  of tool names and narrows the set offered to the underlying
  model on each request to exactly those names. An empty value
  (the default — and what ralph-loops passes when its own Tools
  config is empty) means "offer every tool ikigai-cli ships,"
  matching `claude`'s convention. Tool names are case-sensitive
  and must match the canonical PascalCase names ikigai-cli
  registers (currently `Read` and `Bash`); an unknown name in
  the list is a fatal startup error per R-2247-BPXI whose
  message lists the registered tool names. Whitespace around
  commas is tolerated; empty list elements (e.g. `--tools=,Bash`
  or trailing commas) are tolerated and ignored. The set of
  tools ikigai-cli *can* offer is still a v-bump decision —
  this flag only chooses among the tools the running binary
  already implements; it cannot enable a tool the binary does
  not ship.

- R-ZRRF-LGW1: tool-call and tool-result turns appear on stdout in
  Claude's stream-json shape (`assistant` events containing
  `tool_use` blocks, `user` events containing `tool_result` blocks),
  regardless of how the underlying provider represented them on the
  wire.

- R-AQ6C-0C5B: v1 implements exactly two tools — Read and Bash —
  and offers both to the model on every request. The model can
  perform writes, edits, file discovery, and content search via
  shell commands at a token-cost premium; expanding the tool set
  is a v1.x decision once the agent loop is proven end-to-end.
  Additional tools listed in `tools.md` ship in later versions,
  not v1.

## Stack constraints

- R-10VP-QZBQ: the implementation language is Go. The build artifact
  is a single statically linked binary per supported OS/arch, with
  no runtime dependencies, suitable for dropping next to the real
  `claude` binary on the user's `PATH`.

- R-J5PD-8EBD: the project ships a `Makefile` at the repo root
  with at minimum these targets:
  - `build` (default — runs when `make` is invoked with no args):
    produces the `ikigai-cli` binary at a known path under the
    repo (typical: `./bin/ikigai-cli`).
  - `test`: runs the project's automated test suite and exits
    non-zero on any failure. Tests that exercise the Anthropic
    backend require a real `ANTHROPIC_API_KEY`; the `test` target
    reads the key from `$HOME/.secrets/ANTHROPIC_API_KEY` (a
    single-line file containing the bare key) and exports it
    into the test environment. The key value MUST NOT be echoed,
    logged, included in test output, embedded in error messages,
    written to any file under the repo, or otherwise surfaced
    where it could land in an agent's context, terminal scrollback,
    CI log, or commit. If the key file is absent, `test` fails
    with a clear message naming the missing file (not its content)
    rather than running a partial suite or making a real request
    with no auth.
  - `install`: places the built binary at `~/.local/bin/ikigai-cli`,
    creating the directory if it does not exist. Users are
    expected to have `~/.local/bin` on their `PATH`.
  - `clean`: removes build artifacts produced by `build`.
  Targets are independent — `test` and `install` may depend on
  `build`, but `clean` must not depend on a build step.

- R-BUFE-M5E0: all meaningful behavior lives in importable Go
  packages. The `cmd/ikigai-cli` binary is a thin wiring layer that
  parses flags, hooks stdin/stdout/signals, and delegates to those
  packages. No package outside `cmd/` may read `os.Args`, touch
  `os.Stdin`/`os.Stdout` directly, or install signal handlers — the
  agent loop, tool runtime, provider clients, and wire-format codec
  must each operate over caller-supplied `io.Reader`/`io.Writer`,
  `context.Context`, and configuration values. This is forward-
  looking: ralph-loops is expected to eventually link the same
  packages in-process and skip the subprocess boundary entirely.


## Out of scope for v1

- R-1O1T-0MEX: there is no permission system. ikigai-cli accepts
  `--dangerously-skip-permissions` as a no-op flag and behaves as if
  permissions were always skipped. No prompts, no allow/deny logic.

- R-2DNP-1SZI: MCP servers are not exposed as tools.
  `ENABLE_CLAUDEAI_MCP_SERVERS` and `CLAUDE_CONFIG_DIR` are accepted
  and ignored — consistent with the configless rule below.

- R-321O-P7TE: subagent / Task tool, NotebookEdit, background-bash
  lifecycle (BashOutput, KillBash), SlashCommand, Skill, WebFetch,
  WebSearch, and TodoWrite are not part of v1. They may be added in
  later tiers per `tools.md` once the core agent loop is proven.

## Companion specs

- `tools.md` — the tiered list of tool implementations, one
  requirement per tool, with semantic detail (Read line-numbering,
  Edit uniqueness guardrail, Bash timeout/foreground policy, etc.).
- `wire-format.md` — the exact shape of every stream-json event
  ikigai-cli must accept on stdin or emit on stdout, derived from
  ralph-loops' actual usage of `claude`.
- `providers.md` — per-provider mapping notes: model identifier
  grammar, effort vocabulary per model, tool-use translation, and
  HTTP/SSE specifics.
