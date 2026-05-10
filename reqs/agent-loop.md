# Agent loop

This file specifies ikigai-cli's runtime behavior: how the binary
turns a single ralph-loops invocation into one provider iteration
that streams stream-json events to stdout and terminates with a
single `result` event.

The wire shape of those events is pinned in `wire-format.md`; the
provider-side request/response translation is pinned in
`providers.md`; the tool implementations are pinned in `tools.md`.
This file is about the loop that ties them together.

## Wiring

- R-U1M3-IGO7: `cmd/ikigai-cli` reads the user event(s) supplied
  on stdin, assembles a provider request whose model is the
  resolved `--model`, whose tool list is the subset of
  `tools.All()` selected by `--tools` per R-YFCR-J9IL (an empty
  `--tools` value, the default, means the entire `tools.All()`
  surface), whose system prompt is the framing prompt (per
  R-8PF6-I8FP), and whose initial message history is the user
  turn read from stdin, then drives the agent loop against the
  resolved provider and writes every produced stream-json event
  to stdout. Exactly one `result` event terminates the iteration
  (per R-VJBZ-S578). This is the wiring that turns ikigai-cli
  from a flag validator into a usable drop-in for ralph-loops.

## Tool round-trip

- R-8293-8LCI: when an assistant turn ends with a `tool_use` stop
  reason, the agent loop dispatches every `tool_use` block in
  that turn through `internal/tools`, emits a user event whose
  blocks are exactly one `tool_result` per `tool_use` (correlated
  by id per R-5DMN-M3F2, all answered before the next `result`
  per R-5ZKU-HYRK), appends both the assistant turn and the
  tool-result user turn to the conversation history (preserving
  any thinking / reasoning blocks per R-ROBI-V64M), and re-invokes
  the provider with the appended history. The iteration only
  emits a `result` event when the assistant terminates on a
  non-tool stop reason. Without this, no ralph iteration can
  complete because ralph's iterations universally call tools.

## System prompt

- R-8PF6-I8FP: ikigai-cli sends a non-empty system prompt on
  every provider request. The prompt establishes that the model
  is operating inside an agentic loop, that the tools advertised
  in the request's tool list are available to call, and that the
  model should use them as needed before producing its final
  answer. Exact wording is an implementation choice and may
  evolve. Without this orientation the model behaves as a
  chatbot rather than an agent: tool-use never fires and no
  iteration completes. Structured-output guidance is layered on
  top of this baseline per R-WFWM-BKWX where applicable, with
  the specific shape pinned by R-GA6J-9O0I.

- R-GA6J-9O0I: the framing system prompt (R-8PF6-I8FP) must
  instruct the model that its final answer is a single bare JSON
  value — not wrapped in a markdown code fence, not preceded or
  followed by explanatory prose, and with nothing after it. Exact
  wording is an implementation choice; the load-bearing property
  is that the instruction be present and unambiguous. Without it,
  models that default to chat-style formatting (notably Gemini)
  emit ```` ```json {...} ``` ```` and the structured-output
  parser rejects every turn, driving each iteration into
  R-WFWM-BKWX's bounded retries even when the JSON itself is
  well-formed. The fix lives in the prompt rather than in a
  fence-tolerant parser because tolerating fences would mask
  further drift in what models emit; failing loudly at the parse
  step keeps the contract honest.

- R-XQHM-7TKL: when an iteration is invoked with a `--json-schema`
  per cli-surface.md R-JNEB-EVLU, the agent loop populates
  `provider.Request.ResponseSchema` with the schema's raw bytes on
  every `client.Stream` call within that iteration — the initial
  dispatch and every tool-result follow-up. Forwarding the schema
  engages the provider-native structured-output mode (OpenAI's
  `text.format.json_schema` per providers.md R-3Z86-0IPP, Google's
  `responseJsonSchema` per providers.md R-PP17-XGH5), which is the
  load-bearing first line of defense against shape drift. The
  local validation per providers.md R-WFWM-BKWX continues to run
  as the second line of defense, catching cases where a provider
  silently degrades structured-output enforcement (notably Google
  when tools are also active in the same request, per
  providers/google.md §12). This requirement exists because the
  `ResponseSchema` field is defined on `provider.Request` and the
  providers correctly read it, but without an explicit assignment
  in the agent loop the runtime call sites pass `nil`, leaving
  every structured-output requirement above as dead code.
