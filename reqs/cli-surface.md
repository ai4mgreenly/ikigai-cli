# CLI surface

This file pins how `ikigai-cli` presents itself on the command
line: flag form, help output, and other human-visible aspects of
the CLI itself. The drop-in invariant for ralph-loops is
R-YARD-835I; this file covers properties beyond the bare
invocation contract.

## Help output

- R-XV7A-7AEF: `ikigai-cli --help` renders every flag in double-
  dash form (`--model`, `--effort`, `--json-schema`,
  `--dangerously-skip-permissions`). The single-dash invocation
  form may continue to be accepted, but it must not appear in the
  help output. This matches `claude --help` and the convention the
  spec itself uses, so a human comparing the two binaries sees a
  consistent surface.

## Required ralph-loops flags

- R-6TC0-ZSKM: ikigai-cli accepts both `-p` (short form) and
  `--print` (long form) on the command line, matching the
  upstream `claude` CLI's print-mode flag pair. The two forms
  are equivalent; `-p` is the short alias for `--print`.
  ralph-loops invokes the binary with `-p` as part of the
  drop-in flag set (R-YARD-835I), and operators invoking
  ikigai-cli by hand may type either form. Print mode is the
  only invocation mode ikigai-cli supports, so the flag has no
  behavioral effect beyond being accepted without error.
  Rejecting either form with "flag provided but not defined"
  violates the drop-in invariant and prevents any iteration
  from running.
  In `--help` output (per R-XV7A-7AEF, which requires double-
  dash form), this flag renders as `--print` with `-p` noted as
  the short alias — not as `--p`. The single-letter short form
  must not be auto-rendered as a fake long form.

- R-Z4YN-KG36: ikigai-cli accepts the remaining flags ralph-loops
  passes on every invocation, even though each is effectively a
  no-op for ikigai-cli:
  - `--verbose` — ikigai-cli has no verbosity dial; accepted and
    ignored.
  - `--input-format stream-json` — ikigai-cli only ever consumes
    stream-json on stdin (per wire-format.md); the flag pins the
    only supported value and is accepted as confirmation.
  - `--output-format stream-json` — same shape on the output
    side; ikigai-cli only ever emits stream-json (R-UXDS-W9UQ).
  - `--replay-user-messages` — ralph-loops uses this flag to ask
    `claude` to echo each user-message turn back on the output
    stream so the orchestrator can render its own view of the
    conversation. ikigai-cli already emits `user` events for
    every user-message turn it processes (per wire-format.md and
    R-UXDS-W9UQ), so the flag is accepted and has no additional
    behavioral effect.
  Rejecting any of these as "flag provided but not defined"
  violates the drop-in invariant (R-YARD-835I).
  Note: `--tools` is no longer a no-op flag; its value is
  honored per R-YFCR-J9IL (narrows the tool surface offered to
  the model). ralph-loops still passes it empty by default,
  which means "all tools ikigai-cli ships," so the visible
  behavior under default ralph-loops invocations is unchanged.
  The complete ralph-loops flag set is pinned by R-6TC0-ZSKM
  (`-p`), R-YFCR-J9IL (`--tools`), and this requirement
  together; if ralph-loops adds a new flag in the future, a
  corresponding requirement must be added here before ikigai-
  cli is expected to remain a drop-in.
