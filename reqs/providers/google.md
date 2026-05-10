# Google Generative Language API — implementation reference

This is implementation-grade reference data for the Google
Generative Language API (`generativelanguage.googleapis.com`)
surface ikigai-cli will call directly over HTTPS. The high-level
requirements (which models to support, effort vocabulary, auth
conventions, etc.) live in `../providers.md`. This file pins the
wire-level shapes the build agent needs to construct requests and
parse responses.

ikigai-cli uses the **Generative Language API** (AI Studio key
surface), NOT Vertex AI. Verified against `ai.google.dev` docs as
of 2026-05-10.

## 1. Endpoint and auth

- **Streaming endpoint**: `POST /v1beta/{model=models/*}:streamGenerateContent`
  - Concrete form: `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:streamGenerateContent`
- **Unary endpoint** (not used in v1, listed for completeness):
  `POST /v1beta/models/{model}:generateContent`
- **Query params**:
  - `alt=sse` — switches the response to Server-Sent Events
    (`data: {json}\n\n`). Without it, `streamGenerateContent`
    returns a JSON array of chunks instead of SSE. ikigai-cli
    always sends `alt=sse`.
- **Required headers** (verbatim):
  ```
  x-goog-api-key: <GOOGLE_API_KEY>
  content-type: application/json
  ```
- **Auth header**: `x-goog-api-key` (NOT `Authorization: Bearer`,
  which is Vertex / OAuth only). Read the key from env var
  `GOOGLE_API_KEY`; pass its value as the header. The legacy
  `?key=...` query parameter still works but is no longer the
  documented form, and starting **June 19, 2026** Google will
  block "unrestricted traffic keys", so always send a restricted
  key in the header.

```
POST /v1beta/models/gemini-3.1-pro-preview:streamGenerateContent?alt=sse
x-goog-api-key: $GOOGLE_API_KEY
content-type: application/json
```

[generate-content reference](https://ai.google.dev/api/generate-content) ·
[Using Gemini API keys](https://ai.google.dev/gemini-api/docs/api-key)

## 2. Request body shape

Top-level fields on `GenerateContentRequest`:

```json
{
  "contents":          [ /* Content */ ],
  "systemInstruction": { "parts": [{"text": "..."}] },
  "tools":             [ /* Tool */ ],
  "toolConfig":        { /* ToolConfig */ },
  "generationConfig":  { /* GenerationConfig */ },
  "safetySettings":    [ /* SafetySetting */ ],
  "cachedContent":     "cachedContents/{id}"
}
```

ikigai-cli sends `contents`, `systemInstruction`, `tools`,
`toolConfig`, `generationConfig`, and `safetySettings` on every
request. `cachedContent` is not sent in v1 (per
`../providers.md` R-T5EY-Y23Z).

`Content`:

```json
{ "role": "user" | "model", "parts": [ /* Part */ ] }
```

`Part` is a oneof — exactly one data field per part, plus
optional `thought` / `thoughtSignature` / `partMetadata`:

```json
{ "text": "..." }
{ "inlineData":          { "mimeType": "image/png", "data": "<base64>" } }
{ "fileData":            { "mimeType": "...",       "fileUri": "..." } }
{ "functionCall":        { "id": "...", "name": "...", "args": {...} } }
{ "functionResponse":    { "id": "...", "name": "...", "response": {...} } }
{ "text": "...", "thought": true, "thoughtSignature": "<opaque>" }
```

The historical `role: "function"` for `functionResponse`-bearing
turns is still accepted but the current docs use `role: "user"`;
ikigai-cli sends `role: "user"`.

[generate-content reference (Content/Part)](https://ai.google.dev/api/generate-content)

## 3. Tool definition shape

```json
{
  "tools": [{
    "functionDeclarations": [{
      "name": "get_weather",
      "description": "Retrieves current weather for the given location.",
      "parameters": {
        "type": "object",
        "properties": { "city": { "type": "string" } },
        "required": ["city"]
      }
    }]
  }],
  "toolConfig": {
    "functionCallingConfig": {
      "mode": "AUTO",
      "allowedFunctionNames": []
    }
  }
}
```

`functionDeclarations[].parameters` accepts the OpenAPI 3.0
schema subset. Constructs that the subset does not support
(`$ref`, `oneOf` with discriminator, recursive schemas,
`patternProperties`, schema-valued `additionalProperties`,
`allOf` composition) are rejected by the backend at startup per
`../providers.md` R-OEP1-E6AR rather than passed through.

`functionCallingConfig.mode` values:
```
"AUTO"        // default; model decides whether to call a tool
"ANY"         // model must call some tool
"NONE"        // model must not call any tool
"VALIDATED"   // server-side schema enforcement (Gemini 3.x); not used in v1
```

ikigai-cli sends `mode: "AUTO"` whenever tools are supplied per
`../providers.md` R-TTSY-LGXV.

[Function calling guide](https://ai.google.dev/gemini-api/docs/function-calling)

## 4. Streaming response (SSE)

With `?alt=sse`, the body is a stream of
`data: {GenerateContentResponse}\n\n` events. There is no
terminating `data: [DONE]` sentinel; the server simply closes the
stream when finished.

Each chunk:

```json
{
  "candidates": [{
    "content":       { "role": "model", "parts": [ /* Part */ ] },
    "finishReason":  "STOP" | "MAX_TOKENS" | "SAFETY" | "RECITATION"
                   | "OTHER" | "BLOCKLIST" | "PROHIBITED_CONTENT"
                   | "FINISH_REASON_UNSPECIFIED",
    "index": 0,
    "safetyRatings": [...],
    "citationMetadata": {...},
    "groundingMetadata": {...},
    "tokenCount": 0
  }],
  "promptFeedback": {...},
  "usageMetadata":  { /* see §8 — present on the final chunk */ },
  "modelVersion":   "...",
  "responseId":     "..."
}
```

Streaming behaviour:
- **Text** parts arrive incrementally — successive chunks each
  contain a `text` part with the next fragment.
- **Thought-summary** parts (`"thought": true`) arrive
  incrementally as rolling summaries during generation, but only
  when `generationConfig.thinkingConfig.includeThoughts: true`.
  ikigai-cli sends `false` per `../providers.md` R-QKQL-VHR7,
  so these parts do not appear on the wire.
- **`functionCall`** parts arrive complete in a single chunk
  (args are not streamed token-by-token); a chunk may contain
  multiple `functionCall` parts for parallel calls.
- **`thoughtSignature`** may appear on any `model`-role part
  (text, functionCall) regardless of `includeThoughts`. Every
  such signature must be preserved and echoed back per
  `../providers.md` R-P1V4-NTDY.
- The **final chunk** carries `finishReason` on the candidate
  and `usageMetadata` at the top level; `usageMetadata` does
  not appear on intermediate chunks.

In-stream errors arrive as a `data:` chunk whose payload carries
an `error` key in place of `candidates` — see §10. Clients must
check each chunk for an `error` key before treating it as a
`GenerateContentResponse`.

[generate-content reference (Candidate, GenerateContentResponse)](https://ai.google.dev/api/generate-content) ·
[Thinking guide](https://ai.google.dev/gemini-api/docs/thinking)

## 5. Thinking / effort

Path: `generationConfig.thinkingConfig`.

```json
"generationConfig": {
  "thinkingConfig": {
    "thinkingLevel":   "low" | "medium" | "high",
    "includeThoughts": false
  }
}
```

Gemini 3.x uses **`thinkingLevel`** (categorical). Gemini 2.5's
integer **`thinkingBudget`** still works for back-compat but is
mutually exclusive with `thinkingLevel`; sending both yields a
400. ikigai-cli sends only `thinkingLevel`.

Per-model effort rules for the MVP-supported model:

| Model | `thinkingLevel` values | Default | Disable thinking |
|---|---|---|---|
| `gemini-3.1-pro-preview` | `low | medium | high` | `medium` (pinned by registry per R-M1C2-M8E5) | not supported (`none`/`minimal` rejected at startup) |

`includeThoughts` controls whether human-readable thought
summaries appear as `text` parts with `"thought": true` on the
streamed output. **It does not control `thoughtSignature`** —
signatures arrive on parts regardless. ikigai-cli sends
`includeThoughts: false`.

**Critical**: `thoughtSignature` is an opaque, encrypted, tamper-
checked blob attached to `model`-role parts (text and
functionCall). Subsequent requests in the same iteration must
echo every signature back on the same parts in the same order
or the model loses reasoning continuity (analogous to
Anthropic's `signature` on `thinking` blocks and OpenAI's
`encrypted_content` on `reasoning` items). See `../providers.md`
R-P1V4-NTDY.

[Thinking guide](https://ai.google.dev/gemini-api/docs/thinking)

## 6. Structured output

```json
"generationConfig": {
  "responseMimeType":   "application/json",
  "responseJsonSchema": { /* draft-2020-12 JSON Schema */ }
}
```

Gemini 3.x supports `responseJsonSchema` (draft-2020-12,
including `$ref`); ikigai-cli always uses this mode and never
the legacy OpenAPI-3 `responseSchema`. The two are mutually
exclusive — sending both yields a 400.

The schema passed via `--json-schema` (per OVERVIEW
R-JNEB-EVLU) is forwarded verbatim into `responseJsonSchema`.
Schemas that use keywords Google does not support yield a 400
that surfaces as an iteration error per `../providers.md`
R-E2W7-K5JB without translation; repairing the schema is the
operator's responsibility.

The constrained output appears as `text` parts on the final
`model`-role candidate; the assembled string is parsable JSON
conforming to the schema.

[Structured output guide](https://ai.google.dev/gemini-api/docs/structured-output)

## 7. Tool call result round-trip

When the model emits a `functionCall` part, send the result back
as the next `Content` entry with `role: "user"` containing one
or more `functionResponse` parts:

```json
{
  "role": "user",
  "parts": [{
    "functionResponse": {
      "id":       "<id from the functionCall>",
      "name":     "get_weather",
      "response": { "temperature_c": 21, "condition": "clear" }
    }
  }]
}
```

- **IDs**: Gemini 3.x emits a unique `id` for every
  `functionCall`; the matching `functionResponse.id` **must** be
  echoed so the model can correlate parallel calls.
- The `response` value is a free-form object; wrap raw
  scalars / strings in `{"result": "..."}`.
- Multiple parallel calls in one model turn → one user turn
  with multiple `functionResponse` parts in original arrival
  order.
- The preceding `model`-role turn (with its `functionCall`
  parts and any `thoughtSignature` values) **must be retained
  verbatim** in `contents`, per `../providers.md` R-P1V4-NTDY.

[Function calling guide](https://ai.google.dev/gemini-api/docs/function-calling)

## 8. Safety settings

ikigai-cli sends `safetySettings` setting `BLOCK_NONE` on every
documented harm category:

```json
"safetySettings": [
  {"category": "HARM_CATEGORY_HARASSMENT",        "threshold": "BLOCK_NONE"},
  {"category": "HARM_CATEGORY_HATE_SPEECH",       "threshold": "BLOCK_NONE"},
  {"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "BLOCK_NONE"},
  {"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
  {"category": "HARM_CATEGORY_CIVIC_INTEGRITY",   "threshold": "BLOCK_NONE"}
]
```

Rationale and the policy boundary live at `../providers.md`
R-SFT2-WVJE.

[Safety settings reference](https://ai.google.dev/gemini-api/docs/safety-settings)

## 9. Usage metadata

On the final SSE chunk:

```json
"usageMetadata": {
  "promptTokenCount":         123,   // includes cached tokens
  "cachedContentTokenCount":  100,   // present only when caching was hit
  "candidatesTokenCount":     45,    // includes thoughtsTokenCount
  "thoughtsTokenCount":       30,    // present when thinking is on
  "toolUsePromptTokenCount":  12,    // present when tools are exercised
  "totalTokenCount":          168,
  "promptTokensDetails":      [ {"modality": "TEXT", "tokenCount": 123} ],
  "cacheTokensDetails":       [...],
  "candidatesTokensDetails":  [...]
}
```

Field presence: `cachedContentTokenCount`, `thoughtsTokenCount`,
and `toolUsePromptTokenCount` only appear when the corresponding
feature is actually exercised. `usageMetadata` does not appear
on intermediate streamed chunks; accumulate from the final
chunk only.

Mapping onto the Claude Code-shaped `usage` is specified at
`../providers.md` R-NNV8-Z7ZH.

[generate-content reference (UsageMetadata)](https://ai.google.dev/api/generate-content)

## 10. Error response shape

Standard Google API error envelope (HTTP status mirrors
`error.code`):

```json
{
  "error": {
    "code":    400,
    "message": "Invalid JSON payload received. ...",
    "status":  "INVALID_ARGUMENT",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.BadRequest",
        "fieldViolations": [
          { "field": "contents[0].parts[0]", "description": "..." }
        ]
      }
    ]
  }
}
```

Common `status` values: `INVALID_ARGUMENT` (400),
`FAILED_PRECONDITION` (400, e.g. billing not enabled),
`UNAUTHENTICATED` (401), `PERMISSION_DENIED` (403), `NOT_FOUND`
(404, e.g. unknown model ID), `RESOURCE_EXHAUSTED` (429,
quota / rate limit), `INTERNAL` (500), `UNAVAILABLE` (503),
`DEADLINE_EXCEEDED` (504).

**Streaming error semantics**: an error before the SSE stream
opens returns the JSON envelope above with the corresponding
HTTP status. An error after the stream has begun arrives as a
regular `data:` SSE chunk whose JSON payload carries an `error`
key (same envelope) in place of `candidates`. Per
`../providers.md` R-UFR5-HCAD, every chunk is checked for an
`error` key before being treated as a `GenerateContentResponse`.

ikigai-cli classifies the `status` enum into the
`provider.ErrorKind` taxonomy per `../providers.md` R-R7WP-54UE
and surfaces them per R-E2W7-K5JB. Raw HTTP status codes and
response bodies do not leak onto stdout.

Rate-limit headers: standard Google quota headers (`Retry-After`
on 429) are present but the exact set is not enumerated in the
public reference. Treat HTTP 429 + `RESOURCE_EXHAUSTED` as the
canonical rate-limit signal and honor `Retry-After` if present.

[Errors reference](https://ai.google.dev/gemini-api/docs/troubleshooting)

## 11. Pricing reference (gemini-3.1-pro-preview)

Public pricing as of 2026-05-10, paid tier, USD per 1M tokens.
This is the data the model registry per `../providers.md`
R-ZZLK-I9CK and R-V2X8-QZDK must declare for
`gemini-3.1-pro-preview`:

| Dimension | ≤200K input | >200K input |
|---|---|---|
| Input | $2.00 | $4.00 |
| Output (incl. thinking tokens) | $12.00 | $18.00 |
| Cached input (read) | $0.20 | $0.40 |
| Cache creation | n/a — explicit caching not used in v1 | n/a |

The 200K input-token threshold is the registry's tier breakpoint
per R-V2X8-QZDK. Cache storage ($4.50 / 1M tokens / hour) is not
billed per request and is not modeled in the registry.

[Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing)

## 12. Markdown fence handling

Gemini wraps structured free-form output (JSON, code, single-line
status payloads) in markdown code fences with high frequency, even
against an explicit "raw output" instruction in the prompt. There is
no documented `generationConfig` parameter to suppress this in
free-form mode; the `responseJsonSchema` mode (§ structured output,
`../providers.md` R-PP17-XGH5) does guarantee fence-free output but
applies only when a schema is supplied.

For the un-schema'd free-form case ikigai-cli applies two layers per
`../providers.md` R-K8MR-FN4P and R-2WLP-5VTQ:

1. **Prompt augmentation** — append a fixed sentence to
   `systemInstruction` instructing the model to emit raw text with
   no fences. Soft signal; reduces but does not eliminate fences.

2. **Outer-fence stripping** — at the boundary where the assistant
   turn's accumulated text becomes a stdout `TextBlock`, detect the
   shape

   ```
   ```<lang?>
   <body>
   ```
   ```

   wrapping the entire payload (first non-whitespace line opens,
   last non-whitespace line closes, no other unbalanced fence
   between) and emit `<body>` in place of the original text.
   Mixed prose-and-fence, multiple top-level fences, and
   unbalanced fences are emitted unchanged — only the unambiguous
   "the whole response is a fenced block" case is rewritten.

Both layers run unconditionally — they are NOT skipped when
`responseJsonSchema` is in effect. Empirically, Gemini's
structured-output mode does not bind output cleanly when
function-calling tools are also active in the same request
(the documented schema-plus-tools interaction), and ralph's
iterations always combine schema with tools. Under genuine
fence-free structured-output mode the strip pass is a no-op
anyway, so always-running is both safer and simpler than
conditional gating.

## Notes / unconfirmed

- Whether a date-stamped variant like
  `gemini-3.1-pro-preview-MM-DD` exists as an alias — public
  model card lists only the bare ID and the `-customtools`
  variant.
- Whether `thoughtsTokenCount` is reliably populated on
  streaming for 3.1 Pro — at least one open developer-forum
  report has it missing on `gemini-3-flash-preview`; behavior on
  3.1 Pro is not separately confirmed.
- Precise SSE error-chunk shape mid-stream is documented in the
  troubleshooting guide but not in the primary streaming
  reference; the `{"error": {...}}` envelope is treated as
  authoritative based on Google's documented error shape.
- Full set of rate-limit response headers is not publicly
  enumerated. Treat HTTP 429 + `RESOURCE_EXHAUSTED` as the
  canonical signal.
