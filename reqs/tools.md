# Tools

This file specifies the tools ikigai-cli implements and exposes to
the underlying model. v1 shipped exactly two — Read and Bash (per
R-AQ6C-0C5B) — with the model performing writes, edits, file
discovery, and content search via shell commands at a token-cost
premium. v1.x expands the surface one tool at a time, in this
order: Write, Edit, Glob, Grep. The Grep tool diverges
visibly from Claude Code's behavior on regex syntax (RE2
rather than PCRE — see R-PHCN-83RU); every other tool
matches Claude Code's observable shape. Each addition is a deliberate
amendment of R-AQ6C-0C5B's "v1 = Read + Bash" rule and requires
the corresponding section below plus an update to the scope
membership test (`internal/scope/tools_membership_test.go`).

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

- R-EBGD-2Z08: `Bash` captures the subprocess's stdout and stderr
  as separate streams internally, even though the model-facing
  `tool_result` content combines them per R-IR21-6UNB. The split
  capture exists so the wire-level sidecar (R-DPI6-73NQ) can
  preserve the distinction for downstream consumers that render
  stderr differently from stdout (notably ralph-loops' Bash
  renderer, which dims stderr). The combined model-facing string
  is rebuilt from the two streams; what the model sees is
  unchanged.

- R-DPI6-73NQ: `Bash` `tool_result` user events carry a
  `tool_use_result` sidecar (per wire-format.md R-CZWA-5X35) of
  shape
  `{"stdout": "<string>", "stderr": "<string>", "interrupted": <bool>}`.
  `stdout` and `stderr` are the captured streams from the
  subprocess (each capped at the same per-stream limit as the
  combined model-facing output, so a single runaway stream cannot
  bypass R-LXOL-5ACL via the sidecar); `interrupted` is `true`
  when the per-invocation timeout fired (R-JWIM-71UX) and the
  process group was killed before completion, `false` otherwise.
  The shape matches Claude Code CLI's Bash sidecar so a downstream
  renderer keyed on that shape behaves the same whether the
  underlying engine is Claude or ikigai-cli.

## Write

- R-W0PK-4XBN: the `Write` tool writes textual content to a file
  at an absolute `file_path`. On success the file's bytes are
  exactly the bytes the model supplied as `content` — no added
  trailing newline, no line-ending normalization, no encoding
  conversion. The model is the sole authority on the file's
  contents.

- R-JR8E-92QM: the input JSON schema declares two properties,
  both required: `file_path` (string, absolute path to the file)
  and `content` (string, the content to write). Shape matches
  Claude Code's Write tool. There is no append mode, no encoding
  selector, no mode/permission selector, no "create only if
  missing" flag — a model that has used Claude Code's `Write`
  must be able to call ikigai-cli's version with the same
  arguments per R-YNXM-CVXI.

- R-7VLS-NHCD: `Write` overwrites the existing file at
  `file_path` if one is present. The previous contents are
  discarded without backup or confirmation. ikigai-cli does NOT
  enforce a "Read before Write" precondition at the tool layer;
  that gate is harness UX in Claude Code, not part of the tool's
  observable semantics, and replicating it here would diverge
  from a clean overwrite contract that the model can reason
  about uniformly.

- R-K3XF-PB1Y: when `file_path`'s parent directory does not
  exist, `Write` returns an error `tool_result` per R-ZUM3-QUVT.
  It does not auto-create intermediate directories. The model is
  expected to use `Bash` (`mkdir -p`) when it needs to materialize
  a new directory tree. Failing loudly here is preferred over
  silently materializing structure the model did not ask for.

- R-CMWG-58TR: writes are atomic with respect to concurrent
  readers — `Write` either replaces the file's contents fully or
  leaves the prior contents in place. The implementation writes
  to a temporary file in the same directory as `file_path` and
  renames it over the target. A failure mid-write does not leave
  a partially-written or truncated target file. The temporary
  file is removed on any error path before the error
  `tool_result` is returned.

- R-2DHN-A6VK: the materialized file (whether newly created or
  replacing an existing file) has mode 0644. The temp+rename
  strategy of R-CMWG-58TR does not preserve the prior file's
  mode; preserving mode is not a guarantee `Write` makes. Models
  needing a different mode use `Bash` (`chmod`) after the write.

- R-PE9Q-1JFL: on success `Write` returns a `tool_result` with
  `is_error: false` and a short human-readable confirmation that
  distinguishes file creation from file replacement and includes
  the absolute path (e.g. `File created successfully at
  /abs/path` or `File updated successfully at /abs/path`). The
  shape matches Claude Code's success message so a model trained
  against it parses the result the same way.

## Glob

- R-Q4PX-7KJN: the `Glob` tool finds files whose paths match a
  shell-style glob pattern. The input JSON schema declares
  `pattern` (required string, the glob) and `path` (optional
  string, an absolute directory to search from). Shape matches
  Claude Code's Glob tool per R-YNXM-CVXI.

- R-NMVL-8WCR: when `path` is omitted, `Glob` searches from the
  session cwd (the working directory ikigai-cli was launched
  in), consistent with `Bash`'s cwd convention (R-KM4I-88FI).
  When supplied, `path` must be absolute (R-0GKA-MQ8B) and must
  be an existing directory; otherwise an error `tool_result` is
  returned per R-ZUM3-QUVT.

- R-3BHF-CKQT: pattern syntax is the standard glob with `*`
  (any characters within a single path segment), `?` (one
  character), `[...]` (character class), and `**` (recursive
  descent across zero or more directory segments). A pattern
  containing `**` may match files at any depth below the search
  root; a pattern without `**` matches only at the depths
  implied by its literal segments. This matches the syntax a
  model trained against Claude Code's Glob expects.

- R-Y8ZE-5DPM: `Glob` returns regular files only — directories,
  symlinks-to-directories, sockets, and other non-regular
  entries are excluded from the result even when the pattern
  matches their name. The result body is a newline-separated
  list of absolute paths, sorted by file modification time
  descending (newest first). Sort order is load-bearing: it
  matches Claude Code's convention so the model surfaces
  recently-touched files at the top of its context, which is
  almost always what makes a Glob useful.

- R-LGRA-2VTW: every path in the result is absolute, regardless
  of whether the search root was the default cwd or a supplied
  `path`. The model never has to resolve a relative path
  returned by `Glob` against an implicit working directory.

- R-X7CN-9MFB: an empty match set is a successful (`is_error:
  false`) `tool_result` with a short body indicating no matches
  (e.g. `No files found`). A pattern that does not match is
  not an error — it is data the model may legitimately want to
  act on (e.g. confirming a file does not exist).

- R-EJSO-1HUK: the result is capped at 100 matched paths. When
  the cap is reached, the truncation is visible to the model as
  a notice appended to the output (the same loud-truncation
  posture as `Bash` per R-LXOL-5ACL); the model is not silently
  given a partial set. Models needing the full set narrow the
  pattern or split the search across subdirectories.

## Grep

- R-W2KP-TYRG: the `Grep` tool searches file contents for a
  regular-expression pattern across one or more files. The
  input JSON schema declares `pattern` (required string, the
  regex) and the optional properties `path` (absolute file or
  directory to search), `glob` (file-name glob filter, e.g.
  `*.go`), `type` (named file-type filter, see R-T1YQ-V7BN),
  `output_mode` (enum `content` | `files_with_matches` |
  `count`, default `files_with_matches`), `-i` (boolean,
  case-insensitive matching), `-n` (boolean, prefix matching
  lines with their line number — only meaningful with
  `output_mode=content`), `-A` / `-B` / `-C` (integer context
  lines after / before / around each match — only meaningful
  with `output_mode=content`), `multiline` (boolean, see
  R-A8DI-RLSF), and `head_limit` (integer max output entries).
  Shape matches Claude Code's Grep tool per R-YNXM-CVXI.

- R-DV6B-4XLA: when `path` is omitted, `Grep` searches from the
  session cwd, consistent with `Bash` (R-KM4I-88FI) and `Glob`
  (R-NMVL-8WCR). When supplied, `path` must be absolute
  (R-0GKA-MQ8B) and must exist; it may be either a single
  regular file (searched directly) or a directory (searched
  recursively). A non-existent path or a relative path returns
  an error `tool_result` per R-ZUM3-QUVT.

- R-PHCN-83RU: regex syntax is **Go's RE2** (the stdlib
  `regexp` package), not PCRE. RE2 supports literal strings,
  character classes, alternation, quantifiers, anchors,
  capturing groups, and the `(?i)` / `(?s)` / `(?m)` inline
  flags; it does NOT support lookaround, backreferences, or
  recursive patterns. A pattern that fails to compile under
  RE2 returns an error `tool_result` containing the compiler's
  message — the model is told why and reformulates. This is a
  deliberate divergence from Claude Code's ripgrep-backed Grep,
  forced by R-10VP-QZBQ (ikigai-cli ships as a single
  statically linked binary with no runtime dependencies, so
  shelling out to or bundling ripgrep is not an option). The
  schema match still preserves model fluency for the common
  case (literal strings, simple regex), which covers the
  overwhelming majority of real Grep calls.

- R-MO9F-AKEW: `output_mode` selects the result shape.
  `files_with_matches` (the default) returns one absolute path
  per file containing at least one match, newline-separated.
  `content` returns matching lines, each prefixed with the
  matched file's absolute path and a colon (with line numbers
  inserted when `-n` is true, and surrounding context lines
  inserted when `-A` / `-B` / `-C` are non-zero). `count`
  returns one `<absolute-path>:<count>` line per file with at
  least one match. The shapes match Claude Code's Grep output
  modes so the model parses results identically.

- R-Z5JW-CG1H: the `glob` filter restricts the file walk to
  paths whose final segment matches the supplied glob (e.g.
  `*.go` matches every `.go` file regardless of directory;
  `**/*.test.ts` matches `.test.ts` files at any depth). When
  both `glob` and `type` are supplied, a file must satisfy
  both filters to be searched (intersection, not union).

- R-T1YQ-V7BN: the `type` filter accepts a short language name
  and resolves it to a fixed set of file extensions. The
  supported set is: `go` (.go), `py` (.py, .pyw), `js` (.js,
  .mjs, .cjs), `ts` (.ts, .tsx), `rust` (.rs), `java` (.java),
  `c` (.c, .h), `cpp` (.cc, .cpp, .cxx, .hpp, .hxx), `md`
  (.md, .markdown), `sh` (.sh, .bash), `json` (.json), `yaml`
  (.yaml, .yml), and `toml` (.toml). An unknown `type` name
  returns an error `tool_result` listing the supported names.
  This map is intentionally narrower than ripgrep's full type
  list — ikigai-cli covers the common languages a model
  actually reaches for; rare types are addressed via `glob`.

- R-A8DI-RLSF: `multiline` true compiles the pattern with the
  RE2 flags `(?s)` (dotall — `.` matches newline) and `(?m)`
  (multiline — `^` and `$` match at line boundaries within the
  input) and feeds each file's full contents to the matcher
  rather than scanning line-by-line, allowing a single match
  to span multiple lines. Default false. Combining `multiline`
  with `output_mode=content` returns each match's full span as
  a single (potentially multi-line) entry.

- R-NRBC-OMJ6: `head_limit` truncates the output to the first
  N entries — N lines for `content`, N paths for
  `files_with_matches`, N `path:count` rows for `count`. When
  truncation occurs (whether from `head_limit` or from the
  default per-call output cap of 50000 bytes), a notice is
  appended to the output making the truncation visible to the
  model, matching the loud-truncation posture of `Bash`
  (R-LXOL-5ACL) and `Glob` (R-EJSO-1HUK). The 50KB default cap
  exceeds `Bash`'s 30KB because content-mode greps legitimately
  produce more output than a typical shell command and clipping
  too aggressively forces wasteful re-greps.

- R-K4UP-DSXC: directory traversal skips hidden entries (any
  path segment beginning with `.`) and a fixed denylist of
  common build / vendor / cache directories: `node_modules`,
  `vendor`, `dist`, `build`, `target`, `.venv`, `venv`,
  `__pycache__`. There is no flag to override these skips —
  models needing to inspect them use `Bash` (`ls -la`,
  `cat .gitignore`, etc). The skip set is deliberately fixed
  and small rather than reading `.gitignore`: it covers the
  noise that swamps almost every real grep, without dragging
  in gitignore-parsing complexity, and the behavior matches
  closely enough to ripgrep's defaults that a model trained
  against Claude Code's Grep gets equivalent results for the
  common case.

- R-MGY7-WFLI: an empty match set is a successful (`is_error:
  false`) `tool_result` with a short body indicating no matches
  (e.g. `No matches found`). A pattern that matches nothing is
  not an error — it is information the model may legitimately
  want to act on.

## Edit

- R-RTG4-Q9VK: the `Edit` tool performs an exact-string
  replacement within a single file at an absolute `file_path`.
  The input JSON schema declares `file_path` (required string,
  absolute path), `old_string` (required string, the literal
  text to find), `new_string` (required string, the literal
  replacement text), and `replace_all` (optional boolean,
  default false). Shape matches Claude Code's Edit tool per
  R-YNXM-CVXI. There is no regex mode, no fuzzy-match mode, no
  multi-edit batching — multiple independent edits in one file
  are issued as separate `Edit` calls (a future `MultiEdit`
  tool may batch them).

- R-8XBN-MP2C: `file_path` must be absolute (R-0GKA-MQ8B) and
  must exist as a regular file. A relative path, a non-existent
  path, or a path naming a directory or non-regular entry
  returns an error `tool_result` per R-ZUM3-QUVT. `Edit` does
  not create files — that is `Write`'s job.

- R-LFJD-7HRO: replacement is byte-exact: `Edit` matches and
  substitutes `old_string` and `new_string` as literal byte
  sequences with no whitespace normalization, no line-ending
  fuzzing, no encoding conversion, no leading-indentation
  inference. The model is the sole authority on the bytes that
  enter and leave the file. This is load-bearing: a tool that
  silently "fixes up" the model's strings would make every
  failed match a guessing game about what the tool transformed,
  which models trained against Claude Code's Edit are not
  prepared for.

- R-3CWS-EAYI: when `replace_all` is false (the default),
  `old_string` must occur exactly once in the file's contents.
  Zero occurrences returns an error `tool_result` whose body
  reports that the string was not found. More than one
  occurrence returns an error `tool_result` whose body reports
  the occurrence count and instructs the model to either
  enlarge `old_string` with more surrounding context to make
  the match unique or set `replace_all=true`. The
  exactly-once gate matches Claude Code's Edit semantics and
  prevents the model from accidentally clobbering the wrong
  occurrence.

- R-VK0M-5BTL: when `replace_all` is true, every occurrence of
  `old_string` in the file's contents is replaced with
  `new_string`. Zero occurrences is still an error per
  R-3CWS-EAYI's not-found clause — `replace_all=true` relaxes
  the uniqueness constraint, not the must-match-something
  constraint. The success result reports the number of
  replacements performed.

- R-NJZH-1XPE: `new_string` must differ from `old_string`. A
  call where the two are byte-equal returns an error
  `tool_result` reporting the no-op rather than silently
  rewriting the file with identical contents. Catching this
  loudly surfaces model mistakes (e.g. forgetting to actually
  modify the replacement text) at the call site rather than as
  a confusing later "the edit didn't take" symptom.

- R-O6QT-FAUR: `old_string` must be non-empty. An empty
  `old_string` returns an error `tool_result`. An empty
  `old_string` would match at every byte boundary in the file
  and is almost certainly a model bug; refusing is preferable
  to performing a destructive replacement that no one intended.

- R-DM8K-9SCG: `Edit` is atomic with respect to concurrent
  readers, by the same temp-file-plus-rename mechanism as
  `Write` (R-CMWG-58TR). The temporary file is created in the
  same directory as `file_path`, the modified contents are
  written into it, and it is renamed over the target. A
  failure mid-write does not leave a partially-edited target
  file, and the temporary file is removed on any error path
  before the error `tool_result` is returned.

- R-HEFY-4WJN: `Edit` preserves the target file's mode across
  the edit — the post-edit file has the same permission bits
  the pre-edit file had. This diverges deliberately from
  `Write`'s mode policy (R-2DHN-A6VK, which materializes new
  files at 0644): `Edit` modifies an existing file's contents,
  and the file's mode is part of its identity, not something
  the call is asking to reset. Implementation stats the
  pre-edit file, chmod's the temporary file to match before the
  rename, and the rename promotes the matched mode atomically.

- R-MZWT-K8VR: ikigai-cli does NOT enforce a "Read before Edit"
  precondition at the tool layer, by the same logic as `Write`
  (R-7VLS-NHCD). The byte-exact uniqueness gate of R-3CWS-EAYI
  already forces the model to know the file's contents well
  enough to construct a uniquely-matching `old_string`; a
  separate Read-first gate would add ceremony without adding
  safety, and would diverge from the clean tool-layer contract
  the rest of `tools.md` maintains.

- R-PQXB-J5LC: on success `Edit` returns a `tool_result` with
  `is_error: false` and a short human-readable confirmation
  including the absolute path and the number of replacements
  performed (e.g. `File edited successfully at /abs/path (1
  replacement)` or `... (3 replacements)`). The shape matches
  Claude Code's success message so a model trained against it
  parses the result the same way.

