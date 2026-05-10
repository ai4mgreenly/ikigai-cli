// Package google implements [provider.Client] against the Google
// Generative Language API (AI Studio surface).
//
// R-JVAI-4WXP: this backend posts to
// https://generativelanguage.googleapis.com/v1beta/models/{model}:streamGenerateContent?alt=sse
// using Server-Sent Events for streaming. The non-streaming
// :generateContent endpoint is not used.
//
// R-KIGL-EK0W: authentication uses GOOGLE_API_KEY passed as the
// x-goog-api-key request header. No OAuth, no Vertex AI routing.
package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	"github.com/ai4mgreenly/ikigai-cli/internal/trace"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com"
	v1betaModels   = "/v1beta/models/"
	streamSuffix   = ":streamGenerateContent"
)

// Client is the Google Generative Language API backend.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
	// R-92NN-7DNI: optional trace writer; nil means no tracing.
	tracer *trace.Tracer
}

// SetTracer attaches t to the client.
func (c *Client) SetTracer(t *trace.Tracer) {
	c.tracer = t
}

// SetBaseURL overrides the default API base URL. Used in tests to redirect
// to a local httptest server.
func (c *Client) SetBaseURL(u string) {
	c.baseURL = u
}

// New constructs a [Client]. apiKey must be the value of the
// GOOGLE_API_KEY env var; an empty key is rejected so that a missing
// credential surfaces here rather than as a 401 from the API.
func New(apiKey, model string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("google: GOOGLE_API_KEY is required")
	}
	if model == "" {
		return nil, fmt.Errorf("google: model is required")
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		http:    &http.Client{},
	}, nil
}

// Stream issues a streaming Generative Language API call and returns a
// channel of normalized [provider.Event] values. The channel closes when
// the stream ends (cleanly or otherwise); on a non-2xx HTTP response
// Stream returns a typed *[provider.Error] before any goroutine starts.
//
// R-JVAI-4WXP: POST .../streamGenerateContent?alt=sse.
func (c *Client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	// R-M1C2-M8E5: default effort "medium"; the driver should apply
	// DefaultEffort first, but guard here for safety.
	effort := req.Effort
	if effort == "" {
		effort = "medium"
	}

	payload, err := buildPayload(c.model, effort, req)
	if err != nil {
		return nil, &provider.Error{Kind: provider.ErrInvalidRequest, Msg: err.Error()}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &provider.Error{Kind: provider.ErrInvalidRequest, Msg: "marshal request"}
	}

	endpoint := c.baseURL + v1betaModels + c.model + streamSuffix + "?alt=sse"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &provider.Error{Kind: provider.ErrInvalidRequest, Msg: "build request"}
	}
	// R-KIGL-EK0W: x-goog-api-key header, NOT Authorization: Bearer.
	httpReq.Header.Set("x-goog-api-key", c.apiKey)
	httpReq.Header.Set("content-type", "application/json")

	c.tracer.LogRequest(httpReq.Method, httpReq.URL.String(), httpReq.Header, body)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, mapTransportError(err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		c.tracer.LogResponse(resp.StatusCode, resp.Header, errBody)
		return nil, mapErrorBody(resp.StatusCode, errBody)
	}

	c.tracer.LogResponse(resp.StatusCode, resp.Header, nil)

	out := make(chan provider.Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		p := &sseParser{ctx: ctx, out: out, tracer: c.tracer}
		p.run(resp.Body)
	}()
	return out, nil
}

// buildPayload translates a [provider.Request] into the GenerateContentRequest
// JSON body.
//
// R-RTUW-106W: system prompt via systemInstruction.
// R-SFT2-WVJE: safetySettings BLOCK_NONE on all 5 harm categories.
// R-M1C2-M8E5 / R-QKQL-VHR7: thinkingConfig with thinkingLevel and includeThoughts:false.
// R-T5EY-Y23Z: cachedContent is not sent.
// R-TTSY-LGXV / R-OEP1-E6AR: tools and toolConfig when tools are present.
// R-PP17-XGH5: responseJsonSchema when a schema is supplied.
func buildPayload(model, effort string, req provider.Request) (map[string]any, error) {
	payload := map[string]any{
		"contents":        translateMessages(req.Messages),
		"safetySettings":  safetySettings(),
		"generationConfig": buildGenerationConfig(effort, req.ResponseSchema),
	}
	if req.SystemPrompt != "" {
		// R-RTUW-106W: system prompt via systemInstruction, not a user turn.
		payload["systemInstruction"] = map[string]any{
			"parts": []any{map[string]any{"text": req.SystemPrompt}},
		}
	}
	if len(req.Tools) > 0 {
		decls, err := translateTools(req.Tools)
		if err != nil {
			return nil, err
		}
		payload["tools"] = []any{map[string]any{
			"functionDeclarations": decls,
		}}
		// R-TTSY-LGXV: mode AUTO when tools are supplied.
		payload["toolConfig"] = map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "AUTO",
			},
		}
	}
	// R-T5EY-Y23Z: cachedContent is intentionally omitted.
	return payload, nil
}

// safetySettings returns BLOCK_NONE on every documented harm category.
// R-SFT2-WVJE.
func safetySettings() []any {
	categories := []string{
		"HARM_CATEGORY_HARASSMENT",
		"HARM_CATEGORY_HATE_SPEECH",
		"HARM_CATEGORY_SEXUALLY_EXPLICIT",
		"HARM_CATEGORY_DANGEROUS_CONTENT",
		"HARM_CATEGORY_CIVIC_INTEGRITY",
	}
	out := make([]any, len(categories))
	for i, cat := range categories {
		out[i] = map[string]any{"category": cat, "threshold": "BLOCK_NONE"}
	}
	return out
}

// buildGenerationConfig constructs the generationConfig field.
// R-M1C2-M8E5: thinkingLevel is the effort string ("low"|"medium"|"high").
// R-QKQL-VHR7: includeThoughts is always false.
// R-PP17-XGH5: responseMimeType + responseJsonSchema when schema is set.
func buildGenerationConfig(effort string, responseSchema json.RawMessage) map[string]any {
	cfg := map[string]any{
		"thinkingConfig": map[string]any{
			"thinkingLevel":   effort,
			"includeThoughts": false,
		},
	}
	if len(responseSchema) > 0 {
		cfg["responseMimeType"] = "application/json"
		// R-PP17-XGH5: forward verbatim into responseJsonSchema (draft-2020-12).
		cfg["responseJsonSchema"] = responseSchema
	}
	return cfg
}

// translateMessages converts provider.Messages to Google's contents array.
// It first builds an ID→name map from assistant ToolUseBlocks so that
// ToolResultBlocks (which lack a Name field) can supply the function name
// required by Google's functionResponse.
func translateMessages(msgs []provider.Message) []any {
	nameForID := make(map[string]string)
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant {
			for _, b := range m.Blocks {
				if tu, ok := b.(provider.ToolUseBlock); ok {
					nameForID[tu.ID] = tu.Name
				}
			}
		}
	}

	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			out = append(out, translateUserMessage(m.Blocks, nameForID))
		case provider.RoleAssistant:
			out = append(out, translateAssistantMessage(m.Blocks))
		}
	}
	return out
}

// translateUserMessage converts user-turn blocks to a Google Content with
// role "user". TextBlock → text Part; ToolResultBlock → functionResponse Part.
func translateUserMessage(blocks []provider.Block, nameForID map[string]string) any {
	parts := make([]any, 0, len(blocks))
	for _, b := range blocks {
		switch v := b.(type) {
		case provider.TextBlock:
			parts = append(parts, map[string]any{"text": v.Text})
		case provider.ToolResultBlock:
			// Google requires the function name on the functionResponse.
			name := nameForID[v.ToolUseID]
			var response any
			if v.IsError {
				response = map[string]any{"error": v.Content}
			} else {
				response = map[string]any{"result": v.Content}
			}
			parts = append(parts, map[string]any{
				"functionResponse": map[string]any{
					"id":       v.ToolUseID,
					"name":     name,
					"response": response,
				},
			})
		}
	}
	return map[string]any{"role": "user", "parts": parts}
}

// translateAssistantMessage converts assistant-turn blocks to a Google Content
// with role "model". ThinkingBlock with empty Text carries a pending
// thoughtSignature that is attached to the following text or functionCall part.
// This is how thoughtSignatures are round-tripped per R-P1V4-NTDY.
//
// R-P1V4-NTDY: ThinkingBlock{Text:"", Sig:sig} immediately before a TextBlock
// or ToolUseBlock means that part had a thoughtSignature on the previous turn.
// Replay it verbatim so the model can correlate reasoning state.
func translateAssistantMessage(blocks []provider.Block) any {
	parts := make([]any, 0, len(blocks))
	var pendingSig string

	for _, b := range blocks {
		switch v := b.(type) {
		case provider.ThinkingBlock:
			if v.Text == "" {
				// Pending thoughtSignature for the next adjacent part.
				pendingSig = v.Signature
			} else {
				// Text content paired with a thoughtSignature (e.g. a text part
				// that carried thoughtSignature on a previous turn, stored with
				// non-empty Text in ThinkingBlock).
				p := map[string]any{"text": v.Text}
				if v.Signature != "" {
					p["thoughtSignature"] = v.Signature
				}
				parts = append(parts, p)
				pendingSig = ""
			}

		case provider.TextBlock:
			p := map[string]any{"text": v.Text}
			if pendingSig != "" {
				p["thoughtSignature"] = pendingSig
				pendingSig = ""
			}
			parts = append(parts, p)

		case provider.ToolUseBlock:
			input := json.RawMessage(v.Input)
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			fc := map[string]any{
				"id":   v.ID,
				"name": v.Name,
				"args": input,
			}
			p := map[string]any{"functionCall": fc}
			if pendingSig != "" {
				p["thoughtSignature"] = pendingSig
				pendingSig = ""
			}
			parts = append(parts, p)
		}
	}
	return map[string]any{"role": "model", "parts": parts}
}

// translateTools converts neutral tool descriptors to Google's
// functionDeclarations shape. R-OEP1-E6AR: schemas using constructs outside
// the OpenAPI 3.0 subset are rejected here rather than at Google's API layer.
func translateTools(tools []provider.Tool) ([]any, error) {
	out := make([]any, 0, len(tools))
	for _, t := range tools {
		schema := json.RawMessage(t.InputSchema)
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		} else {
			if err := validateSchemaForGoogle(schema, t.Name); err != nil {
				return nil, err
			}
		}
		decl := map[string]any{
			"name":       t.Name,
			"parameters": schema,
		}
		if t.Name != "" {
			// description is optional; omit when absent.
		}
		out = append(out, decl)
	}
	return out, nil
}

// validateSchemaForGoogle rejects schema constructs outside the OpenAPI 3.0
// subset accepted by Google's functionDeclarations[].parameters.
// R-OEP1-E6AR: $ref, oneOf, patternProperties, schema-valued
// additionalProperties, and allOf are unsupported.
func validateSchemaForGoogle(raw json.RawMessage, toolName string) error {
	var node map[string]any
	if err := json.Unmarshal(raw, &node); err != nil {
		return fmt.Errorf("tool %q: invalid schema: %w", toolName, err)
	}
	return validateSchemaNode(node, toolName)
}

func validateSchemaNode(node map[string]any, toolName string) error {
	for _, key := range []string{"$ref", "oneOf", "allOf", "patternProperties"} {
		if _, ok := node[key]; ok {
			return fmt.Errorf("tool %q: schema uses unsupported construct %q (not in OpenAPI 3.0 subset accepted by Google)", toolName, key)
		}
	}
	// schema-valued additionalProperties (object, not boolean)
	if ap, ok := node["additionalProperties"]; ok {
		if _, isMap := ap.(map[string]any); isMap {
			return fmt.Errorf("tool %q: schema uses unsupported construct \"additionalProperties\" as schema object (not in OpenAPI 3.0 subset accepted by Google)", toolName)
		}
	}
	// Recurse into properties values.
	if props, ok := node["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				if err := validateSchemaNode(sub, toolName); err != nil {
					return err
				}
			}
		}
	}
	// Recurse into array items.
	if items, ok := node["items"].(map[string]any); ok {
		if err := validateSchemaNode(items, toolName); err != nil {
			return err
		}
	}
	return nil
}

// sseParser turns Google's SSE data chunks into normalized [provider.Event]
// values. Google SSE uses only data: lines (no event: type); each data:
// payload is a complete GenerateContentResponse JSON. The stream ends when
// the server closes the connection — there is no [DONE] sentinel.
type sseParser struct {
	ctx        context.Context
	out        chan<- provider.Event
	hadToolUse bool
	stopReason string
	usage      *usageMeta
	errored    bool
	tracer     *trace.Tracer
}

// usageMeta holds the fields from the final chunk's usageMetadata.
// R-NNV8-Z7ZH.
type usageMeta struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
}

func (p *sseParser) emit(e provider.Event) bool {
	select {
	case p.out <- e:
		return true
	case <-p.ctx.Done():
		return false
	}
}

func (p *sseParser) run(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var data strings.Builder

	flush := func() {
		if data.Len() == 0 {
			return
		}
		s := data.String()
		p.tracer.LogSSEPair("", s)
		p.handleChunk(s)
		data.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimPrefix(rest, " "))
			continue
		}
		// Ignore event:, id:, retry: (Google SSE doesn't use them meaningfully).
	}
	flush()

	if p.errored {
		return
	}

	// Emit usage from final chunk then signal completion.
	if p.usage != nil {
		// R-NNV8-Z7ZH: promptTokenCount includes cached tokens; subtract to
		// get the non-cached input count; cachedContent is the cache_read count.
		// CacheCreationInputTokens is always 0 for Google (no per-request creation).
		p.emit(provider.EventUsage{
			InputTokens:          p.usage.PromptTokenCount - p.usage.CachedContentTokenCount,
			OutputTokens:         p.usage.CandidatesTokenCount,
			CacheReadInputTokens: p.usage.CachedContentTokenCount,
		})
	}

	sr := p.stopReason
	if sr == "" {
		if p.hadToolUse {
			sr = "tool_use"
		} else {
			sr = "end_turn"
		}
	}
	p.emit(provider.EventDone{StopReason: sr})
}

// handleChunk processes one SSE data payload (a complete GenerateContentResponse).
func (p *sseParser) handleChunk(data string) {
	// R-UFR5-HCAD: check for top-level error key before treating as a
	// GenerateContentResponse.
	var envelope struct {
		Error      *googleError `json:"error"`
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text             string          `json:"text"`
					Thought          bool            `json:"thought"`
					ThoughtSignature string          `json:"thoughtSignature"`
					FunctionCall     *functionCall   `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *usageMeta `json:"usageMetadata"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return
	}

	if envelope.Error != nil {
		// R-UFR5-HCAD: in-stream error chunk; terminate without EventDone.
		p.errored = true
		return
	}

	for _, cand := range envelope.Candidates {
		for _, part := range cand.Content.Parts {
			// R-QKQL-VHR7: skip thought: true parts (thinking summaries).
			if part.Thought {
				continue
			}

			if part.FunctionCall != nil {
				// R-P1V4-NTDY: emit thoughtSignature BEFORE EventToolUse so the
				// agent loop's drainTurn places ThinkingBlock before ToolUseBlock
				// in providerBlocks. translateAssistantMessage reads this ordering
				// to reconstruct the functionCall part with thoughtSignature.
				if part.ThoughtSignature != "" {
					p.emit(provider.EventThinking{Text: "", Signature: part.ThoughtSignature})
				}
				args := part.FunctionCall.Args
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				// R-MTDR-EYG4: functionCall part → EventToolUse.
				p.hadToolUse = true
				p.emit(provider.EventToolUse{
					ID:    part.FunctionCall.ID,
					Name:  part.FunctionCall.Name,
					Input: args,
				})
			} else if part.Text != "" {
				// R-P1V4-NTDY: emit thoughtSignature BEFORE EventTextDelta so
				// drainTurn's flushText() fires on the EventThinking, placing
				// ThinkingBlock before TextBlock in providerBlocks. The ordering
				// lets translateAssistantMessage attach the sig to the text part.
				if part.ThoughtSignature != "" {
					p.emit(provider.EventThinking{Text: "", Signature: part.ThoughtSignature})
				}
				p.emit(provider.EventTextDelta{Text: part.Text})
			}
		}

		// Track finishReason from the last candidate in the final chunk.
		if cand.FinishReason != "" && cand.FinishReason != "FINISH_REASON_UNSPECIFIED" {
			p.stopReason = mapFinishReason(cand.FinishReason, p.hadToolUse)
		}
	}

	if envelope.UsageMetadata != nil {
		p.usage = envelope.UsageMetadata
	}
}

// functionCall is the wire shape of a Google functionCall part.
type functionCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// googleError is the wire shape of a Google API error envelope.
type googleError struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// mapFinishReason translates a Google finishReason string to the normalized
// stop reason the agent loop expects.
func mapFinishReason(reason string, hadToolUse bool) string {
	switch reason {
	case "MAX_TOKENS":
		return "max_tokens"
	case "STOP":
		if hadToolUse {
			return "tool_use"
		}
		return "end_turn"
	default:
		return "end_turn"
	}
}

// mapErrorBody reads an HTTP error response body and returns a typed
// [provider.Error]. R-R7WP-54UE.
func mapErrorBody(statusCode int, body []byte) *provider.Error {
	var resp struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &resp)
	}
	return mapErrorStatus(resp.Error.Status, statusCode)
}

// mapErrorStatus translates a Google error.status string and HTTP status
// into a typed [provider.Error]. R-R7WP-54UE.
func mapErrorStatus(status string, statusCode int) *provider.Error {
	switch status {
	case "UNAUTHENTICATED":
		return &provider.Error{Kind: provider.ErrAuth, Msg: "google rejected credentials"}
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION", "NOT_FOUND", "PERMISSION_DENIED":
		return &provider.Error{Kind: provider.ErrInvalidRequest, Msg: "google rejected the request"}
	case "RESOURCE_EXHAUSTED":
		return &provider.Error{Kind: provider.ErrRateLimit, Msg: "google rate-limited the request"}
	case "DEADLINE_EXCEEDED":
		return &provider.Error{Kind: provider.ErrTimeout, Msg: "google request deadline exceeded"}
	case "INTERNAL", "UNAVAILABLE":
		return &provider.Error{Kind: provider.ErrServer, Msg: "google server error"}
	}
	switch {
	case statusCode == http.StatusUnauthorized:
		return &provider.Error{Kind: provider.ErrAuth, Msg: "google rejected credentials"}
	case statusCode == http.StatusForbidden:
		return &provider.Error{Kind: provider.ErrAuth, Msg: "google rejected credentials"}
	case statusCode == http.StatusTooManyRequests:
		return &provider.Error{Kind: provider.ErrRateLimit, Msg: "google rate-limited the request"}
	case statusCode >= 500:
		return &provider.Error{Kind: provider.ErrServer, Msg: "google server error"}
	case statusCode >= 400:
		return &provider.Error{Kind: provider.ErrInvalidRequest, Msg: "google rejected the request"}
	default:
		return &provider.Error{Kind: provider.ErrUnknown, Msg: "google error"}
	}
}

// mapTransportError converts a Go transport error into a typed [provider.Error].
func mapTransportError(err error) *provider.Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &provider.Error{Kind: provider.ErrTimeout, Msg: "google request deadline exceeded"}
	}
	if errors.Is(err, context.Canceled) {
		return &provider.Error{Kind: provider.ErrUnknown, Msg: "google request canceled"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &provider.Error{Kind: provider.ErrTimeout, Msg: "google request timed out"}
	}
	return &provider.Error{Kind: provider.ErrUnknown, Msg: "google transport failure"}
}
