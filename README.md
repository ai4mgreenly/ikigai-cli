# ikigai-cli

A drop-in replacement for the `claude` CLI that speaks Claude Code's
stream-json protocol on stdin/stdout while transparently running the
conversation against Anthropic, OpenAI, or Google Gemini.

The single load-bearing property: an orchestrator that already drives
`claude` (ralph-loops, in v1) can swap the binary and keep working
without any change to its own code.

## Use it with ralph-loops

[ralph-loops](https://github.com/ai4mgreenly/ralph-loops) is the
orchestrator ikigai-cli is built for. It defaults to spawning `claude`
each iteration, but its `--engine` flag accepts any drop-in
replacement resolved against `$PATH`:

    ralph --engine=ikigai-cli --model=gpt-5.5
    ralph --engine=ikigai-cli --model=gemini-3.1-pro-preview

Install ikigai-cli somewhere on `$PATH` (`make install` puts it at
`~/.local/bin/ikigai-cli`) and ralph will treat it exactly like
`claude` — same stream-json protocol, same flag set, same iteration
contract.

## How it works

ikigai-cli is a translator. It:

1. Reads Claude-shaped `user` stream-json events from stdin.
2. Runs the agent loop against the chosen provider's HTTP API
   (no vendor SDKs — raw HTTP/SSE).
3. Executes built-in tools (Read, Bash, with Write/Edit/Glob/Grep
   on the v1.x roadmap) locally as part of the loop.
4. Emits Claude-shaped `assistant`, `user`, `result`, and `system`
   events back on stdout.
5. Terminates each iteration with exactly one `result` event whose
   `structured_output` satisfies the schema given by `--json-schema`.

Provider selection is inferred from the `--model` prefix:
`claude-*` → Anthropic, `gpt-*` → OpenAI, `gemini-*` → Google.
The matching `<PROVIDER>_API_KEY` environment variable is the only
credential source.

## Usage

```
ikigai-cli -p \
  --model claude-sonnet-4-6 \
  --json-schema '{"type":"object","properties":{"status":{"enum":["DONE","CONTINUE"]}}}' \
  < user-events.jsonl
```

Notable flags:

- `--model` — bare provider model ID, or a provider-defined alias.
- `--effort` — passed through in the provider's native vocabulary;
  not normalized across providers.
- `--json-schema` — the JSON Schema document itself, inline as a
  literal string (not a file path).
- `--tools` — comma-separated allowlist of tool names; empty means
  "every tool this binary ships."
- `--raw` — interleave a debug trace (stdin reads, LLM requests,
  LLM responses) onto stdout. API keys are redacted.
- ralph-loops's flag set (`-p`/`--print`, `--verbose`,
  `--input-format`, `--output-format`, `--replay-user-messages`,
  `--dangerously-skip-permissions`) is accepted; most are no-ops
  preserved purely for drop-in compatibility.

ikigai-cli is **configless**: no config file, no `~/.ikigai/`, no
`IKIGAI_*` env vars. Behavior changes only via CLI flags.

## Repository layout

```
.
├── AGENTS.md          spec-helper instructions (CLAUDE.md → AGENTS.md)
├── reqs/              the specification — source of truth
│   ├── OVERVIEW.md
│   ├── cli-surface.md
│   ├── wire-format.md
│   ├── providers.md (+ providers/)
│   ├── tools.md
│   ├── agent-loop.md
│   └── build.md
└── app-root/          the application source tree (Go)
    ├── AGENTS.md      build-agent's standing instructions
    ├── Makefile
    ├── cmd/ikigai-cli/
    ├── internal/      agent loop, provider clients, tool runtime,
    │                  wire-format codec, model registry
    └── .ralph/        orchestrator state (handoff, verification ledger)
```

The split is deliberate. `reqs/` is human-authored specification — WHAT
and WHY, never HOW. `app-root/` is everything the build agent
(ralph-loops) produces and owns. Each tree has its own `AGENTS.md`.

## Build

From `app-root/`:

- `make` / `make build` — produce `./bin/ikigai-cli`.
- `make test` — run the suite (provider-backend tests require keys at
  `$HOME/.secrets/ANTHROPIC_API_KEY` etc.).
- `make install` — copy the binary to `~/.local/bin/ikigai-cli`.
- `make clean` — remove build artifacts.

The artifact is a single statically linked Go binary with no runtime
dependencies, intended to be dropped next to `claude` on `PATH`.

## Status & scope

v1 targets ralph-loops only. Human-facing UX, IDE integrations, MCP
servers, permission prompts, subagents, NotebookEdit, WebFetch/Search,
and TodoWrite are explicitly out of scope. See `reqs/OVERVIEW.md` for
the full requirement set.
