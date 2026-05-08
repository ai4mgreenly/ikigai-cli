# Tools

This file specifies the tools ikigai-cli implements and exposes to
the underlying model. v1 ships exactly six: Read, Write, Edit, Glob,
Grep, Bash (per OVERVIEW R-AQ6C-0C5B). Future versions add tools
from the deferred list at the bottom.

The unifying principle: **every tool's name, JSON-schema input
shape, and observable result shape match Claude Code's built-in
tool of the same name closely enough that a model trained against
Claude Code uses it correctly without prompting tricks.** That is
load-bearing — drift here makes the model less effective regardless
of which provider answers.

Where Claude Code's behavior has knobs that are clearly out of v1
scope (image input, notebook editing, background processes, plan
mode, sandbox toggles, etc.), this spec calls them out as deferred
rather than reinventing them.

## Cross-cutting requirements

- R-YNXM-CVXI: each tool's exposed name and input JSON schema match
  the corresponding Claude Code built-in tool. A model that has
  used Claude Code's `Read` (or `Bash`, `Edit`, etc.) must be able
  to call ikigai-cli's version with the same arguments and get
  semantically equivalent behavior for the supported subset.

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
  resolution at the tool layer; the model must be aware of the cwd
  it was given via the `system` event at session start.

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
  models trained against it expect line-prefixed output for use
  with `Edit`.

- R-3JJ5-FUPI: `Read` accepts optional `offset` (1-based line
  number to start from) and `limit` (max lines to return)
  arguments. Defaults: start at line 1, return up to 2000 lines.
  Files larger than the limit are truncated; the truncation is
  visible to the model as the absent tail.

- R-46P8-PHSP: an empty existing file returns a system-reminder-
  style notice rather than empty text, so the model can
  distinguish "file is empty" from "read returned nothing."

- R-516Q-9RC2: `Read` of an absolute path that does not exist
  returns an error `tool_result`. It does not auto-create or
  silently return empty text.

- R-5QSM-AXWN: ikigai-cli tracks which files the model has read
  during the current session (a soft "read set"). `Edit` and
  `Write` against a file not in the read set is rejected with an
  error `tool_result` instructing the model to read first. This
  matches Claude Code's read-before-edit guardrail and is
  load-bearing for the model's edit strategy.
  (OPEN: confirm we want this guardrail in v1, or whether v1
  trusts the model to read voluntarily and skips the bookkeeping.)

- R-6CQT-6T95: image and PDF input via `Read` are out of scope for
  v1. A `Read` of a file whose extension indicates such a binary
  type returns an error `tool_result` rather than attempting to
  decode it. Multimodal input may be added in a later version.
  (OPEN: confirm v1 may omit image/PDF reading entirely. Including
  it requires routing multimodal content through each provider's
  vision API, which is non-trivial.)

## Write

- R-6YP0-2OLN: the `Write` tool writes a UTF-8 string to an
  absolute path, creating the file if it does not exist and
  overwriting it if it does.

- R-7KN6-YJY5: `Write` to an existing file requires that file to
  be in the session's read set (per R-5QSM-AXWN). Writing a brand-
  new file does not require a prior `Read`. This matches Claude
  Code's behavior and prevents the model from blind-overwriting
  files it has not seen.

- R-85DH-GNJY: parent directories of the target path are created
  on demand. `Write` to `/a/b/c/file.txt` succeeds even if `/a/b/c`
  did not previously exist.

- R-8RBO-CIWG: `Write` returns a brief confirmation
  `tool_result` to the model containing the path written and the
  byte count. It does not echo the written content.

## Edit

- R-9D9V-8E8Y: the `Edit` tool replaces an exact substring within
  a file. Inputs: absolute `file_path`, `old_string`, `new_string`,
  optional `replace_all` boolean (default false).

- R-9Z82-49LG: when `replace_all` is false, `old_string` must
  occur exactly once in the file. Zero or multiple matches return
  an error `tool_result` instructing the model to provide more
  context. This is the contract Claude Code's `Edit` enforces and
  the model is trained to handle.

- R-ANM1-ROFC: when `replace_all` is true, every occurrence of
  `old_string` is replaced. Zero matches still return an error.

- R-BAS5-1BIJ: `Edit` requires the target file to be in the
  session's read set (per R-5QSM-AXWN). The same guardrail as
  `Write`.

- R-C41Q-7TB7: `Edit` is byte-exact: it does not normalize line
  endings, trim trailing whitespace, re-indent, or otherwise
  transform `new_string`. A model that supplies content that does
  not match the file's existing whitespace conventions will see
  its supplied content land verbatim.

## Glob

- R-CPZX-3ONP: the `Glob` tool returns a list of file paths
  matching a glob pattern. It supports `**` recursive matching and
  standard glob wildcards (`*`, `?`, `[...]`). Pattern syntax
  matches what Claude Code's `Glob` accepts.

- R-DAQ7-LS9I: `Glob` accepts an optional `path` argument
  (absolute) bounding the search to a subtree. Default is the
  session cwd. Patterns that are themselves absolute paths are
  honored as written.

- R-DXWA-VFCP: results are returned sorted by file modification
  time, newest first. This matches Claude Code's ordering and
  helps the model find recently-touched files first.

- R-EL2E-52FW: result count is capped (initial cap: 100 entries).
  Exceeding the cap returns the first 100 plus a notice that more
  matches exist, rather than truncating silently.

## Grep

- R-F70L-0XSE: the `Grep` tool searches file contents for a
  regular-expression pattern across the cwd subtree (or a
  specified `path`). Pattern grammar matches what Claude Code's
  `Grep` accepts (Rust regex / ripgrep flavor).

- R-G55R-QIJU: `Grep` exposes three result modes via a `mode`
  argument:
  - `content` (default): matching lines with surrounding context
  - `files_with_matches`: list of file paths that contain a match
  - `count`: per-file match counts
  This matches Claude Code's `Grep` output_mode contract.

- R-GR3Y-MDWC: `Grep` accepts standard filter arguments: `glob`
  (restrict to matching filenames), `type` (restrict by file
  type), `-i` (case insensitive), `-n` (line numbers), and
  context flags `-A`, `-B`, `-C` (after, before, context lines).
  Semantics match the corresponding ripgrep flags.

- R-HEA1-W0ZJ: in `content` mode, results are capped (initial cap:
  100 lines). Exceeding the cap returns the first 100 plus a
  notice that more matches exist.

- R-I2O1-JFTF: `Grep` against a path that does not exist returns
  an error `tool_result`, not empty results.

## Bash

- R-IR21-6UNB: the `Bash` tool executes a shell command in a `bash
  -c` subprocess and returns its combined stdout+stderr to the
  model. The command is not parsed or sanitized by ikigai-cli;
  whatever the model supplies is what runs.

- R-JBSB-OY94: `Bash` runs in the foreground only. The model
  receives the result after the command exits. There is no
  background-process support, no `BashOutput`, no `KillBash` in
  v1.
  (OPEN: confirm foreground-only is acceptable for v1, given that
  ralph-loops iterations are expected to complete in bounded time
  and don't need to manage long-running daemons.)

- R-JWIM-71UX: `Bash` enforces a per-invocation timeout. Default:
  120 seconds. The model may request a longer timeout up to a hard
  ceiling of 600 seconds via an optional `timeout` argument
  (milliseconds). Timeouts kill the process group and return an
  error `tool_result` indicating the timeout.
  (OPEN: confirm the defaults of 120s default / 600s ceiling, or
  set different values appropriate for ralph-loops iterations.)

- R-KM4I-88FI: `Bash` runs in the session cwd (the working
  directory ikigai-cli was launched in). The model is expected to
  use absolute paths or explicit `cd` within the command for
  anything outside cwd.

- R-LBQE-9F03: `Bash` returns the command's exit code to the model
  as part of the `tool_result` body, in addition to stdout/stderr.
  Non-zero exit is not itself an `is_error` — it is data the model
  may legitimately want to act on (e.g. test failures).

- R-LXOL-5ACL: `Bash` output is truncated if it exceeds a hard
  byte cap (initial cap: 30000 bytes total combined output). The
  truncation is visible to the model as a notice appended to the
  output, not silent.

## Tools deferred past v1

These are listed for completeness so the v1 boundary is explicit.
None ships in v1.

- R-MOID-K8NV: `TodoWrite` — model-managed task list. Less critical
  for ralph-loops loops than for interactive Claude Code sessions
  but cheap to implement; revisit early in v2.

- R-NGK2-CYPU: `WebFetch` — fetch and extract a URL. High value
  for documentation lookup; defer until v1 is proven.

- R-O2I9-8U2C: `WebSearch` — web search. Lower marginal value if
  `WebFetch` exists; defer.

- R-OOGG-4PEU: `Task` / subagent spawning, `NotebookEdit`,
  `BashOutput` / `KillBash` / background-bash lifecycle,
  `SlashCommand`, `Skill`, `ExitPlanMode`, MCP tool exposure, and
  any harness-specific extensions (Agent, Monitor, Cron*, Team*,
  ScheduleWakeup, etc., visible in some Claude Code deployments)
  are explicitly out of scope for ikigai-cli at any version. They
  are either harness extensions rather than Claude Code built-ins,
  or they require infrastructure (subagents, MCP) far beyond the
  v1 charter.
