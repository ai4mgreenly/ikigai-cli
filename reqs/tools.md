# Tools

This file specifies the tools ikigai-cli implements and exposes to
the underlying model. v1 ships exactly two: Read and Bash (per
R-AQ6C-0C5B). The model handles writes, edits, file
discovery, and content search via shell commands until the tool
set is expanded in a later version.

The unifying principle: **every tool's name, JSON-schema input
shape, and observable result shape match Claude Code's built-in
tool of the same name closely enough that a model trained against
Claude Code uses it correctly without prompting tricks.** That is
load-bearing — drift here makes the model less effective regardless
of which provider answers.

## Cross-cutting requirements

- R-YNXM-CVXI: each tool's exposed name and input JSON schema match
  the corresponding Claude Code built-in tool. A model that has
  used Claude Code's `Read` or `Bash` must be able to call
  ikigai-cli's version with the same arguments and get
  semantically equivalent behavior for the supported subset.
  This Claude-Code-matching shape is the *advertised neutral*
  schema; providers may transform it on the wire to satisfy
  backend-specific constraints (per providers.md R-3959-U3A3).
  The transformation is internal to the backend — the schema
  the tool itself declares stays neutral.

- R-Z8NW-UZJB: tool invocations from the model arrive as
  `tool_use` content blocks in `assistant` events on stdout, and
  ikigai-cli emits the corresponding `tool_result` content blocks
  in `user` events on stdout, in the exact stream-json shape Claude
  Code produces. The wire-level appearance of a tool call to
  ralph-loops is identical regardless of which provider issued it
  underneath.

- R-ZUM3-QUVT: tool failure surfaces to the model as a
  `tool_result` block with `is_error: true` and a human-readable
  text body describing the failure. ikigai-cli does not crash on
  tool errors and does not retry the model with hidden prompts;
  the failure is the model's to handle.

- R-0GKA-MQ8B: every filesystem-touching tool requires absolute
  paths in its arguments. Relative paths are rejected with an
  error `tool_result`. There is no implicit working-directory
  resolution at the tool layer.

- R-12IH-ILKT: tool execution is synchronous from the model's
  point of view — each `tool_use` is followed by exactly one
  `tool_result` before the next assistant turn. ikigai-cli does
  not parallelize tool calls within a single assistant turn in v1,
  even when the underlying provider's API supports parallel tool
  calls.

## Read

- R-21VK-LY2Y: the `Read` tool reads a single file from the local
  filesystem given an absolute path. It returns the file's textual
  content to the model.

- R-2XKY-JZD0: text content is returned in `cat -n` form — each
  line prefixed with its 1-based line number followed by a tab,
  then the line's content. This is the format Claude Code uses;
  models trained against it expect line-prefixed output.

- R-3JJ5-FUPI: `Read` accepts optional `offset` (1-based line
  number to start from) and `limit` (max lines to return)
  arguments. Defaults: start at line 1, return up to 2000 lines.
  Files larger than the limit are truncated; the truncation is
  visible to the model as the absent tail.

- R-516Q-9RC2: `Read` of an absolute path that does not exist
  returns an error `tool_result`. It does not auto-create or
  silently return empty text.

## Bash

- R-IR21-6UNB: the `Bash` tool executes a shell command in a
  `bash -c` subprocess and returns its combined stdout+stderr to
  the model. The command is not parsed or sanitized by ikigai-cli;
  whatever the model supplies is what runs.

- R-JBSB-OY94: `Bash` runs in the foreground only. The model
  receives the result after the command exits.

- R-JWIM-71UX: `Bash` enforces a per-invocation timeout of 120
  seconds. Timeouts kill the process group and return an error
  `tool_result` indicating the timeout.

- R-KM4I-88FI: `Bash` runs in the session cwd (the working
  directory ikigai-cli was launched in). The model is expected to
  use absolute paths or explicit `cd` within the command for
  anything outside cwd.

- R-LBQE-9F03: `Bash` returns the command's exit code to the model
  as part of the `tool_result` body, in addition to stdout/stderr.
  Non-zero exit is not itself an `is_error` — it is data the model
  may legitimately want to act on (e.g. test failures).

- R-LXOL-5ACL: `Bash` output is truncated if it exceeds 30000
  bytes total combined output. The truncation is visible to the
  model as a notice appended to the output, not silent.
