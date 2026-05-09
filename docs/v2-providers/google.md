# Google Generative Language API — implementation reference (v2 design context)

This is implementation-grade reference data for the Google
Generative Language API (`generativelanguage.googleapis.com`)
surface a v2 ikigai-cli would call directly over HTTPS. **It is
design context, not part of the MVP spec.** The high-level v2
design notes (which models to support, effort vocabulary, auth
conventions, etc.) live in `overview.md`. This file pins the wire-
level shapes a future build agent would need to construct requests
and parse responses.

ikigai-cli uses the **Generative Language API**, NOT Vertex AI.
Verified against `ai.google.dev` docs as of 2026-05-08.

## 1. Endpoint and auth

- **Streaming endpoint**: `POST /v1beta/{model=models/*}:streamGenerateContent`
  - Concrete form: `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:streamGenerateContent`
- **Unary endpoint** (for completeness): `POST /v1beta/models/{model}:generateContent`
- **Query params**:
  - `alt=sse` — switches the response to Server-Sent Events
    (`data: {json}\n\n`). Without it, `streamGenerateContent`
    returns a JSON array of chunks instead of SSE.
- **Auth**: pass the API key via the `x-goog-api-key` header. This
  is the method shown in the current API-key docs and the REST
  quickstart. The legacy `?key=...` query parameter still works
  but is no longer the documented form, and starting **June 19,
  2026** Google will block "unrestricted traffic keys", so plan to
  send a restricted key in the header. Use `GOOGLE_API_KEY` from
  env.

```
POST /v1beta/models/gemini-3-pro:streamGenerateContent?alt=sse
x-goog-api-key: $GOOGLE_API_KEY
content-type: application/json
```

[generate-content reference](https://ai.google.dev/api/generate-content) ·
[Using Gemini API keys](https://ai.google.dev/gemini-api/docs/api-key)

## 2. Request body

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

`Content`:

```json
{ "role": "user" | "model" | "function", "parts": [ /* Part */ ] }
```

`Part` is a oneof — exactly one of these data fields per part,
plus optional `thought`/`thoughtSignature`/`partMetadata`:

```json
{ "text": "..." }
{ "inlineData":          { "mimeType": "image/png", "data": "<base64>" } }
{ "fileData":            { "mimeType": "...",       "fileUri": "..." } }
{ "functionCall":        { "id": "...", "name": "...", "args": {...} } }
{ "functionResponse":    { "id": "...", "name": "...", "response": {...} } }
{ "executableCode":      { "id": "...", "language": "PYTHON", "code": "..." } }
{ "codeExecutionResult": { "id": "...", "outcome": "OUTCOME_OK", "output": "..." } }
{ "text": "...", "thought": true, "thoughtSignature": "<opaque>" }
```

`role: "function"` is accepted historically but the current docs
use `role: "user"` for the turn that carries `functionResponse`
parts (see §7).

[generate-content reference (Content/Part)](https://ai.google.dev/api/generate-content) ·
[caching reference (Content schema)](https://ai.google.dev/api/caching)

## 3. Tool definition

```json
{
  "tools": [{
    "functionDeclarations": [{
      "name": "get_weather",
      "description": "...",
      "parameters": {            // OpenAPI 3.0 subset Schema
        "type": "object",
        "properties": { "city": { "type": "string" } },
        "required": ["city"]
      }
    }]
  }],
  "toolConfig": {
    "functionCallingConfig": {
      "mode": "AUTO" | "ANY" | "NONE" | "VALIDATED",
      "allowedFunctionNames": ["get_weather"]
    }
  }
}
```

`VALIDATED` is the new default when functions are combined with
other tools and enforces schema adherence.

[Function calling guide](https://ai.google.dev/gemini-api/docs/function-calling)

## 4. Streaming response (SSE)

With `?alt=sse`, the body is a stream of
`data: {GenerateContentResponse}\n\n` events. There is no
terminating `data: [DONE]`; the server simply closes the stream
when finished.

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
  incrementally as rolling summaries during generation.
- **`functionCall`** parts arrive complete in a single chunk (args
  are not streamed token-by-token); a chunk may contain multiple
  `functionCall` parts for parallel calls.
- The **final chunk** carries `finishReason` on the candidate and
  `usageMetadata` at the top level.

[generate-content reference (Candidate, GenerateContentResponse)](https://ai.google.dev/api/generate-content) ·
[Thinking guide](https://ai.google.dev/gemini-api/docs/thinking)

## 5. Thinking / reasoning parameters

Path: `generationConfig.thinkingConfig`.

```json
"generationConfig": {
  "thinkingConfig": {
    "thinkingLevel":   "minimal" | "low" | "medium" | "high",  // Gemini 3.x
    "thinkingBudget":  -1 | 0 | <int>,                          // Gemini 2.5
    "includeThoughts": true
  }
}
```

- **Gemini 3** uses `thinkingLevel`; default `"high"` (dynamic).
  `"minimal"` is **not** supported on 3.1 Pro.
- **Gemini 2.5** uses `thinkingBudget` integer:
  - 2.5 Flash variants: `0`–`24576` (`0` disables).
  - 2.5 Pro: `128`–`32768` (cannot disable).
  - `-1` = dynamic (default).
- With `includeThoughts: true`, the response contains additional
  `Part`s with `"thought": true` carrying summarized reasoning;
  the actual text lives in `text`. Each thought may carry a
  `thoughtSignature` you echo back in subsequent turns to
  preserve reasoning state.

[Thinking guide](https://ai.google.dev/gemini-api/docs/thinking)

## 6. Structured output

Two mutually exclusive modes under `generationConfig`:

```json
"generationConfig": {
  "responseMimeType": "application/json",
  "responseSchema":     { /* OpenAPI 3.0 Schema subset */ }
}
```
or
```json
"generationConfig": {
  "responseMimeType":     "application/json",
  "responseJsonSchema":   { /* draft-2020-12 JSON Schema, supports $ref */ }
}
```

If `responseJsonSchema` is set, `responseSchema` must be omitted,
and vice versa. `responseJsonSchema` is the newer mode and is
required for advanced features (`$ref`, complex validation).
Supported by Gemini 3.x (Flash-Lite, Pro/Pro-Preview), Gemini 3
Flash Preview, and the entire Gemini 2.5 family. Gemini 2.0 Flash
supports `responseSchema` but requires explicit `propertyOrdering`.

There is no separate "strict" flag — when
`responseMimeType: "application/json"` is set with a schema,
conformance is enforced.

[Structured output guide](https://ai.google.dev/gemini-api/docs/structured-output)

## 7. Tool call result round-trip

When the model emits a `functionCall` part, send the result back as
the next `Content` entry with `role: "user"` containing one or
more `functionResponse` parts:

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

- **IDs**: Gemini 3 model APIs generate a unique `id` for every
  `functionCall`, and you **must** include the matching `id` on
  the `functionResponse` so the model can correlate. For older
  models that do not emit `id`, correlation falls back to `name`
  + ordering.
- The `response` value is a free-form object; wrap raw scalars/
  strings in something like `{"result": "..."}`.
- Multiple parallel calls in one model turn → one user turn with
  multiple `functionResponse` parts (order should match call
  order for safety).
- The historical `role: "function"` is still accepted but
  `"user"` is what the current docs use.

[Function calling guide](https://ai.google.dev/gemini-api/docs/function-calling)

## 8. Usage metadata

On the final SSE chunk:

```json
"usageMetadata": {
  "promptTokenCount":         123,   // includes cached tokens
  "cachedContentTokenCount":  100,   // present only when cachedContent used
  "candidatesTokenCount":     45,    // sum across candidates
  "thoughtsTokenCount":       30,    // present when thinking is on
  "toolUsePromptTokenCount":  12,    // present when tools are exercised
  "totalTokenCount":          168,
  "promptTokensDetails":      [ {"modality": "TEXT", "tokenCount": 123} ],
  "cacheTokensDetails":       [...],
  "candidatesTokensDetails":  [...]
}
```

Field presence: `cachedContentTokenCount`, `thoughtsTokenCount`,
and `toolUsePromptTokenCount` only appear when those features are
actually used.

[generate-content reference (UsageMetadata)](https://ai.google.dev/api/generate-content)

## 9. Cached content

Explicit caching API:

- **Create**: `POST /v1beta/cachedContents` with body
  `{model: "models/...", contents: [...], systemInstruction: {...},
  tools: [...], ttl: "300s" | expireTime: "...", displayName: "..."}`.
  Response includes `name: "cachedContents/{id}"`.
- **Reference**: include `"cachedContent": "cachedContents/{id}"`
  at the top level of the `generateContent`/
  `streamGenerateContent` request. The cached `model` must match
  the request model.
- **Lifecycle**: managed via `cachedContents.get / list / patch /
  delete`; `ttl` or `expireTime` controls expiry.

Implicit caching also exists on Gemini 2.5/3 (no client action
needed — Google automatically caches repeated prefixes and reports
hits in `cachedContentTokenCount`). For ikigai-cli v1, **explicit
caching can be deferred**; rely on implicit caching and observe
`cachedContentTokenCount` to confirm hits.

[Caching reference](https://ai.google.dev/api/caching)

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
`UNAUTHENTICATED` (401), `PERMISSION_DENIED` (403), `NOT_FOUND`
(404), `RESOURCE_EXHAUSTED` (429, quota/rate limit),
`FAILED_PRECONDITION` (400), `INTERNAL` (500), `UNAVAILABLE`
(503), `DEADLINE_EXCEEDED` (504).

**Streaming error semantics**: errors on a stream that has not yet
emitted any data return the JSON envelope above with the
corresponding HTTP status. If the connection has already started
streaming SSE, an error may arrive as an additional `data: {...}`
event whose payload has the same `{"error": {...}}` shape. Clients
must therefore check each SSE chunk for an `error` key before
treating it as a `GenerateContentResponse`.

**Rate-limit headers**: standard Google quota headers (`Retry-After`
on 429) are present but the exact set is not enumerated in the
public reference. Treat HTTP `429` + `RESOURCE_EXHAUSTED` as the
canonical signal and honor `Retry-After` if present.

## Notes / unconfirmed

- Precise SSE error-event shape mid-stream is not in primary docs;
  client behavior was inferred from SDK issue trackers.
- Full set of rate-limit response headers not enumerated publicly.
- Whether `usageMetadata` appears on intermediate chunks is not
  documented — current docs only show it as the final cumulative
  value.
