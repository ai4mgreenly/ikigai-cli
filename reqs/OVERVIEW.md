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

- R-W5A6-O0JQ: ikigai-cli speaks to provider HTTP APIs directly. No
  vendor SDK (Anthropic, OpenAI, Google) may be linked. General-
  purpose libraries — HTTP client, SSE parser, JSON, CLI framework,
  filesystem helpers — are permitted.

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

- R-B9P4-41S7: every tool ikigai-cli implements is offered to the
  underlying model on every request. Ralph-loops never narrows the
  set via `--tools` (it passes the flag empty, which Claude treats
  as "all built-ins"), so the surface ikigai-cli ships *is* the
  surface the model sees. Adding a tool is therefore a v-bump
  decision, not a per-invocation one.

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
    non-zero on any failure.
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
