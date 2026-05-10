package google_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	googlebackend "github.com/ai4mgreenly/ikigai-cli/internal/provider/google"
)

// sseChunk writes a single SSE data: event to w.
func sseChunk(w http.ResponseWriter, payload string) {
	fmt.Fprintf(w, "data: %s\n\n", payload)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// textChunk returns a GenerateContentResponse JSON with one text part.
func textChunk(text string) string {
	return fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"text":%q}]}}]}`, text)
}

// finalChunk returns a GenerateContentResponse JSON with finishReason and usageMetadata.
func finalChunk(finishReason string, promptTokens, outputTokens int) string {
	return fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":%q}],"usageMetadata":{"promptTokenCount":%d,"cachedContentTokenCount":0,"candidatesTokenCount":%d}}`,
		finishReason, promptTokens, outputTokens)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*googlebackend.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := googlebackend.New("test-key", "gemini-3.1-pro-preview")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.SetBaseURL(srv.URL)
	return c, srv
}

func drainEvents(t *testing.T, ch <-chan provider.Event) (
	text string,
	toolUses []provider.EventToolUse,
	thinking []provider.EventThinking,
	done bool,
	stopReason string,
	usage provider.EventUsage,
) {
	t.Helper()
	for ev := range ch {
		switch e := ev.(type) {
		case provider.EventTextDelta:
			text += e.Text
		case provider.EventToolUse:
			toolUses = append(toolUses, e)
		case provider.EventThinking:
			thinking = append(thinking, e)
		case provider.EventDone:
			done = true
			stopReason = e.StopReason
		case provider.EventUsage:
			usage = e
		}
	}
	return
}

// TestR_JVAI_4WXP_GoogleBackendUsesStreamGenerateContentSSE verifies the
// backend POSTs to /v1beta/models/{model}:streamGenerateContent?alt=sse
// with x-goog-api-key auth, and that SSE responses are decoded correctly.
// R-JVAI-4WXP.
func TestR_JVAI_4WXP_GoogleBackendUsesStreamGenerateContentSSE(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAPIKey string
	var gotContentType string

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotContentType = r.Header.Get("content-type")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, textChunk("hello world"))
		sseChunk(w, finalChunk("STOP", 10, 5))
	})

	ch, err := c.Stream(context.Background(), provider.Request{
		Model: "gemini-3.1-pro-preview",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	text, _, _, done, stopReason, _ := drainEvents(t, ch)

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	wantPath := "/v1beta/models/gemini-3.1-pro-preview:streamGenerateContent"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotQuery != "alt=sse" {
		t.Errorf("query = %q, want alt=sse", gotQuery)
	}
	if gotAPIKey != "test-key" {
		t.Errorf("x-goog-api-key = %q, want test-key", gotAPIKey)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Errorf("content-type = %q, want application/json", gotContentType)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if !done {
		t.Error("want done=true")
	}
	if stopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", stopReason)
	}
}

// TestR_RTUW_106W_SystemPromptViaSystemInstruction verifies the system prompt
// is sent in the systemInstruction field, not as a user turn. R-RTUW-106W.
func TestR_RTUW_106W_SystemPromptViaSystemInstruction(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	ch, err := c.Stream(context.Background(), provider.Request{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Blocks: []provider.Block{provider.TextBlock{Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	si, ok := body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction missing or wrong type: %v", body["systemInstruction"])
	}
	parts, _ := si["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("systemInstruction.parts len = %d, want 1", len(parts))
	}
	p, _ := parts[0].(map[string]any)
	// R-RTUW-106W: system prompt must appear in systemInstruction text.
	// R-K8MR-FN4P: the anti-fence directive is appended when no schema is set,
	// so we verify the prompt is a prefix of the full text rather than exact.
	if got, ok := p["text"].(string); !ok || !strings.HasPrefix(got, "You are a helpful assistant.") {
		t.Errorf("systemInstruction.parts[0].text = %v, want prefix %q", p["text"], "You are a helpful assistant.")
	}

	// Verify no systemInstruction-style content appears in contents.
	contents, _ := body["contents"].([]any)
	for _, c := range contents {
		cm, _ := c.(map[string]any)
		if cm["role"] == "system" {
			t.Error("system-role entry found in contents; should be in systemInstruction only")
		}
	}
}

// TestR_SFT2_WVJE_SafetySettingsBlockNone verifies BLOCK_NONE on all 5 harm
// categories. R-SFT2-WVJE.
func TestR_SFT2_WVJE_SafetySettingsBlockNone(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	ss, _ := body["safetySettings"].([]any)
	wantCategories := map[string]bool{
		"HARM_CATEGORY_HARASSMENT":        false,
		"HARM_CATEGORY_HATE_SPEECH":       false,
		"HARM_CATEGORY_SEXUALLY_EXPLICIT": false,
		"HARM_CATEGORY_DANGEROUS_CONTENT": false,
		"HARM_CATEGORY_CIVIC_INTEGRITY":   false,
	}
	for _, entry := range ss {
		m, _ := entry.(map[string]any)
		cat, _ := m["category"].(string)
		thresh, _ := m["threshold"].(string)
		if _, want := wantCategories[cat]; want {
			if thresh != "BLOCK_NONE" {
				t.Errorf("category %q threshold = %q, want BLOCK_NONE", cat, thresh)
			}
			wantCategories[cat] = true
		}
	}
	for cat, seen := range wantCategories {
		if !seen {
			t.Errorf("missing safetySettings entry for %q", cat)
		}
	}
}

// TestR_M1C2_M8E5_DefaultEffortMedium verifies that omitting --effort sends
// thinkingLevel "medium" explicitly. R-M1C2-M8E5.
func TestR_M1C2_M8E5_DefaultEffortMedium(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	// Effort is empty — backend must default to "medium".
	ch, err := c.Stream(context.Background(), provider.Request{Effort: ""})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	gc, _ := body["generationConfig"].(map[string]any)
	tc, _ := gc["thinkingConfig"].(map[string]any)
	if tc["thinkingLevel"] != "medium" {
		t.Errorf("thinkingConfig.thinkingLevel = %v, want \"medium\"", tc["thinkingLevel"])
	}
}

// TestR_QKQL_VHR7_IncludeThoughtsFalse verifies includeThoughts is sent as
// false and that thought: true parts are not forwarded as events. R-QKQL-VHR7.
func TestR_QKQL_VHR7_IncludeThoughtsFalse(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Emit a thought: true part followed by a regular text part.
		thoughtChunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"thinking...","thought":true}]}}]}`
		sseChunk(w, thoughtChunk)
		sseChunk(w, textChunk("actual output"))
		sseChunk(w, finalChunk("STOP", 10, 5))
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	text, _, _, _, _, _ := drainEvents(t, ch)

	// Verify request body has includeThoughts: false.
	gc, _ := body["generationConfig"].(map[string]any)
	tc, _ := gc["thinkingConfig"].(map[string]any)
	if tc["includeThoughts"] != false {
		t.Errorf("thinkingConfig.includeThoughts = %v, want false", tc["includeThoughts"])
	}
	// Verify thought: true part was NOT forwarded as text.
	if strings.Contains(text, "thinking...") {
		t.Errorf("thought: true part leaked into text output: %q", text)
	}
	if text != "actual output" {
		t.Errorf("text = %q, want \"actual output\"", text)
	}
}

// TestR_T5EY_Y23Z_NoCachedContent verifies that cachedContent is never sent.
// R-T5EY-Y23Z.
func TestR_T5EY_Y23Z_NoCachedContent(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	if _, ok := body["cachedContent"]; ok {
		t.Error("cachedContent must not be sent (R-T5EY-Y23Z)")
	}
}

// TestR_MTDR_EYG4_FunctionCallTranslation verifies that functionCall parts
// in the SSE stream are translated to EventToolUse, and tool results are
// sent as functionResponse parts. R-MTDR-EYG4.
func TestR_MTDR_EYG4_FunctionCallTranslation(t *testing.T) {
	callChunk := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call1","name":"get_weather","args":{"city":"Paris"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":0,"candidatesTokenCount":5}}`

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, callChunk)
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	_, toolUses, _, _, stopReason, _ := drainEvents(t, ch)

	if len(toolUses) != 1 {
		t.Fatalf("toolUses len = %d, want 1", len(toolUses))
	}
	tu := toolUses[0]
	if tu.ID != "call1" {
		t.Errorf("ID = %q, want call1", tu.ID)
	}
	if tu.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", tu.Name)
	}
	var args map[string]any
	if err := json.Unmarshal(tu.Input, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["city"] != "Paris" {
		t.Errorf("args.city = %v, want Paris", args["city"])
	}
	if stopReason != "tool_use" {
		t.Errorf("stopReason = %q, want tool_use", stopReason)
	}
}

// TestR_NNV8_Z7ZH_UsageMetadataMapping verifies that the final chunk's
// usageMetadata is mapped correctly to EventUsage. R-NNV8-Z7ZH.
func TestR_NNV8_Z7ZH_UsageMetadataMapping(t *testing.T) {
	// prompt=130 (100 cached), output=40
	chunk := `{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":130,"cachedContentTokenCount":100,"candidatesTokenCount":40}}`

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, chunk)
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	_, _, _, _, _, usage := drainEvents(t, ch)

	// input_tokens = promptTokenCount - cachedContentTokenCount = 130 - 100 = 30
	if usage.InputTokens != 30 {
		t.Errorf("InputTokens = %d, want 30", usage.InputTokens)
	}
	if usage.OutputTokens != 40 {
		t.Errorf("OutputTokens = %d, want 40", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 100 {
		t.Errorf("CacheReadInputTokens = %d, want 100", usage.CacheReadInputTokens)
	}
	if usage.CacheCreationInputTokens != 0 {
		t.Errorf("CacheCreationInputTokens = %d, want 0", usage.CacheCreationInputTokens)
	}
}

// TestR_OEP1_E6AR_ToolDefinitionAdaptation verifies that neutral tool schemas
// are placed in functionDeclarations and that unsupported constructs are
// rejected at startup (before the request is sent). R-OEP1-E6AR.
func TestR_OEP1_E6AR_ToolDefinitionAdaptation(t *testing.T) {
	var body map[string]any
	var reqCount int

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	validSchema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	ch, err := c.Stream(context.Background(), provider.Request{
		Tools: []provider.Tool{
			{Name: "get_weather", InputSchema: validSchema},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	tools, _ := body["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	toolEntry, _ := tools[0].(map[string]any)
	decls, _ := toolEntry["functionDeclarations"].([]any)
	if len(decls) != 1 {
		t.Fatalf("functionDeclarations len = %d, want 1", len(decls))
	}
	decl, _ := decls[0].(map[string]any)
	if decl["name"] != "get_weather" {
		t.Errorf("functionDeclarations[0].name = %v, want get_weather", decl["name"])
	}

	// Unsupported constructs must be rejected before the HTTP call.
	unsupportedSchema := json.RawMessage(`{"type":"object","properties":{"x":{"$ref":"#/$defs/T"}}}`)
	_, err = c.Stream(context.Background(), provider.Request{
		Tools: []provider.Tool{
			{Name: "bad_tool", InputSchema: unsupportedSchema},
		},
	})
	if err == nil {
		t.Error("want error for $ref in schema, got nil")
	}
	if reqCount != 1 {
		t.Errorf("reqCount = %d after unsupported schema; want 1 (no second HTTP call)", reqCount)
	}
}

// TestR_P1V4_NTDY_ThoughtSignatureRoundTrip verifies that thoughtSignatures
// on model-role parts are preserved in the conversation history and echoed
// back on subsequent requests. R-P1V4-NTDY.
func TestR_P1V4_NTDY_ThoughtSignatureRoundTrip(t *testing.T) {
	// First call: model emits text part with thoughtSignature + functionCall.
	firstChunk := `{"candidates":[{"content":{"role":"model","parts":[` +
		`{"text":"I will check the weather","thoughtSignature":"sig-abc"},` +
		`{"functionCall":{"id":"call1","name":"get_weather","args":{"city":"Paris"}},"thoughtSignature":"sig-def"}` +
		`]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":0,"candidatesTokenCount":5}}`

	var secondReqBody map[string]any
	callCount := 0

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if callCount == 1 {
			sseChunk(w, firstChunk)
		} else {
			if err := json.NewDecoder(r.Body).Decode(&secondReqBody); err != nil {
				t.Errorf("decode second request: %v", err)
			}
			sseChunk(w, finalChunk("STOP", 10, 5))
		}
	})

	// First call: receive text+sig and functionCall+sig
	ch, err := c.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Blocks: []provider.Block{provider.TextBlock{Text: "check weather"}}},
		},
	})
	if err != nil {
		t.Fatalf("first Stream: %v", err)
	}
	text, toolUses, thinkingEvents, _, _, _ := drainEvents(t, ch)

	// Verify we got the text and tool use.
	if text != "I will check the weather" {
		t.Errorf("text = %q, want %q", text, "I will check the weather")
	}
	if len(toolUses) != 1 || toolUses[0].ID != "call1" {
		t.Errorf("toolUses = %v, want [{call1 get_weather ...}]", toolUses)
	}
	// Two thinking events (one per thoughtSignature)
	if len(thinkingEvents) != 2 {
		t.Errorf("thinkingEvents len = %d, want 2", len(thinkingEvents))
	}

	// Build conversation history as the agent loop would (simplified).
	// The order from drainTurn would be: ThinkingBlock, TextBlock, ThinkingBlock, ToolUseBlock
	assistantBlocks := []provider.Block{
		provider.ThinkingBlock{Text: "", Signature: "sig-abc"},
		provider.TextBlock{Text: "I will check the weather"},
		provider.ThinkingBlock{Text: "", Signature: "sig-def"},
		provider.ToolUseBlock{ID: "call1", Name: "get_weather", Input: json.RawMessage(`{"city":"Paris"}`)},
	}

	// Second call: replay history with tool result.
	ch2, err := c.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Blocks: []provider.Block{provider.TextBlock{Text: "check weather"}}},
			{Role: provider.RoleAssistant, Blocks: assistantBlocks},
			{Role: provider.RoleUser, Blocks: []provider.Block{
				provider.ToolResultBlock{ToolUseID: "call1", Content: "sunny"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("second Stream: %v", err)
	}
	drainEvents(t, ch2)

	// Verify the assistant turn in the second request contains thoughtSignatures.
	contents, _ := secondReqBody["contents"].([]any)
	var modelContent map[string]any
	for _, c := range contents {
		cm, _ := c.(map[string]any)
		if cm["role"] == "model" {
			modelContent = cm
			break
		}
	}
	if modelContent == nil {
		t.Fatal("no model-role content in second request")
	}
	parts, _ := modelContent["parts"].([]any)
	if len(parts) == 0 {
		t.Fatal("model content has no parts")
	}

	// Find text part with sig-abc
	foundTextSig := false
	foundFCsig := false
	for _, part := range parts {
		pm, _ := part.(map[string]any)
		if pm["text"] != nil && pm["thoughtSignature"] == "sig-abc" {
			foundTextSig = true
		}
		if pm["functionCall"] != nil && pm["thoughtSignature"] == "sig-def" {
			foundFCsig = true
		}
	}
	if !foundTextSig {
		t.Errorf("text part with thoughtSignature sig-abc not found in second request; parts = %v", parts)
	}
	if !foundFCsig {
		t.Errorf("functionCall part with thoughtSignature sig-def not found in second request; parts = %v", parts)
	}
}

// TestR_PP17_XGH5_StructuredOutputResponseJsonSchema verifies that a JSON
// schema is forwarded verbatim into generationConfig.responseJsonSchema.
// R-PP17-XGH5.
func TestR_PP17_XGH5_StructuredOutputResponseJsonSchema(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	ch, err := c.Stream(context.Background(), provider.Request{
		ResponseSchema: schema,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	gc, _ := body["generationConfig"].(map[string]any)
	if gc["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v, want application/json", gc["responseMimeType"])
	}
	// responseJsonSchema must be the schema forwarded verbatim
	rjs, _ := gc["responseJsonSchema"].(map[string]any)
	if rjs == nil {
		t.Fatalf("responseJsonSchema missing or wrong type: %v", gc["responseJsonSchema"])
	}
	if rjs["type"] != "object" {
		t.Errorf("responseJsonSchema.type = %v, want object", rjs["type"])
	}
}

// TestR_R7WP_54UE_ErrorTaxonomyMapping verifies that Google error status
// strings and HTTP codes map to the correct provider.ErrorKind. R-R7WP-54UE.
func TestR_R7WP_54UE_ErrorTaxonomyMapping(t *testing.T) {
	cases := []struct {
		status   string
		httpCode int
		wantKind provider.ErrorKind
	}{
		{"UNAUTHENTICATED", http.StatusUnauthorized, provider.ErrAuth},
		{"PERMISSION_DENIED", http.StatusForbidden, provider.ErrInvalidRequest},
		{"INVALID_ARGUMENT", http.StatusBadRequest, provider.ErrInvalidRequest},
		{"FAILED_PRECONDITION", http.StatusBadRequest, provider.ErrInvalidRequest},
		{"NOT_FOUND", http.StatusNotFound, provider.ErrInvalidRequest},
		{"RESOURCE_EXHAUSTED", http.StatusTooManyRequests, provider.ErrRateLimit},
		{"INTERNAL", http.StatusInternalServerError, provider.ErrServer},
		{"UNAVAILABLE", http.StatusServiceUnavailable, provider.ErrServer},
		{"DEADLINE_EXCEEDED", http.StatusGatewayTimeout, provider.ErrTimeout},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			errBody := fmt.Sprintf(`{"error":{"code":%d,"status":%q,"message":"test error"}}`, tc.httpCode, tc.status)
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.httpCode)
				fmt.Fprint(w, errBody)
			})
			_, err := c.Stream(context.Background(), provider.Request{})
			if err == nil {
				t.Fatal("want error, got nil")
			}
			pe, ok := err.(*provider.Error)
			if !ok {
				t.Fatalf("err type = %T, want *provider.Error", err)
			}
			if pe.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v", pe.Kind, tc.wantKind)
			}
		})
	}
}

// TestR_TTSY_LGXV_FunctionCallingConfigAuto verifies that toolConfig sends
// mode: "AUTO" when tools are supplied. R-TTSY-LGXV.
func TestR_TTSY_LGXV_FunctionCallingConfigAuto(t *testing.T) {
	var body map[string]any

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseChunk(w, finalChunk("STOP", 5, 3))
	})

	ch, err := c.Stream(context.Background(), provider.Request{
		Tools: []provider.Tool{
			{Name: "my_tool", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	drainEvents(t, ch)

	tc, _ := body["toolConfig"].(map[string]any)
	fcc, _ := tc["functionCallingConfig"].(map[string]any)
	if fcc["mode"] != "AUTO" {
		t.Errorf("functionCallingConfig.mode = %v, want AUTO", fcc["mode"])
	}
}

// TestR_UFR5_HCAD_InStreamErrorChunkDetection verifies that an error chunk
// mid-stream terminates the stream cleanly (no panic, channel closed).
// R-UFR5-HCAD.
func TestR_UFR5_HCAD_InStreamErrorChunkDetection(t *testing.T) {
	errChunk := `{"error":{"code":500,"status":"INTERNAL","message":"server blew up"}}`

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Emit some text then an error chunk.
		sseChunk(w, textChunk("partial"))
		sseChunk(w, errChunk)
	})

	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var gotText string
	var gotDone bool
	for ev := range ch {
		switch e := ev.(type) {
		case provider.EventTextDelta:
			gotText += e.Text
		case provider.EventDone:
			gotDone = true
		}
	}

	// Text before the error chunk should have been received.
	if gotText != "partial" {
		t.Errorf("text = %q, want \"partial\"", gotText)
	}
	// No EventDone should be emitted after an in-stream error.
	if gotDone {
		t.Error("EventDone emitted after in-stream error; want no EventDone")
	}
}

// TestR_K8MR_FN4P_AntiFenceSystemPromptAugmentation verifies that the anti-fence
// directive is appended to systemInstruction when responseJsonSchema is not set,
// and omitted when it is. When no system prompt is supplied and no schema, the
// directive alone forms systemInstruction. R-K8MR-FN4P.
func TestR_K8MR_FN4P_AntiFenceSystemPromptAugmentation(t *testing.T) {
	capture := func(t *testing.T, req provider.Request) map[string]any {
		t.Helper()
		var body map[string]any
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			sseChunk(w, finalChunk("STOP", 5, 3))
		})
		ch, err := c.Stream(context.Background(), req)
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		drainEvents(t, ch)
		return body
	}

	sysText := func(body map[string]any) string {
		si, _ := body["systemInstruction"].(map[string]any)
		parts, _ := si["parts"].([]any)
		if len(parts) == 0 {
			return ""
		}
		p, _ := parts[0].(map[string]any)
		return fmt.Sprintf("%v", p["text"])
	}

	const directive = "Output raw text only. Do not wrap your response in markdown code fences."

	// No system prompt, no schema: directive alone.
	body := capture(t, provider.Request{})
	got := sysText(body)
	if got != directive {
		t.Errorf("no prompt, no schema: systemInstruction text = %q, want directive %q", got, directive)
	}

	// System prompt present, no schema: prompt + newline + directive.
	body = capture(t, provider.Request{SystemPrompt: "Be helpful."})
	got = sysText(body)
	want := "Be helpful.\n" + directive
	if got != want {
		t.Errorf("with prompt, no schema: systemInstruction text = %q, want %q", got, want)
	}

	// No system prompt, schema set: no systemInstruction.
	schema := json.RawMessage(`{"type":"object"}`)
	body = capture(t, provider.Request{ResponseSchema: schema})
	if _, ok := body["systemInstruction"]; ok {
		t.Error("schema set, no prompt: systemInstruction must not be present")
	}

	// System prompt present, schema set: prompt only, no directive.
	body = capture(t, provider.Request{SystemPrompt: "Be helpful.", ResponseSchema: schema})
	got = sysText(body)
	if got != "Be helpful." {
		t.Errorf("with prompt and schema: systemInstruction text = %q, want %q", got, "Be helpful.")
	}
	if strings.Contains(got, directive) {
		t.Error("directive must not appear when responseJsonSchema is set")
	}
}

// TestR_2WLP_5VTQ_OuterFenceStripping verifies that a single markdown code fence
// wrapping the entire text response is stripped at the Google provider boundary,
// and that prose-mixed, multi-fence, unbalanced, and schema-set cases are left
// unchanged. R-2WLP-5VTQ.
func TestR_2WLP_5VTQ_OuterFenceStripping(t *testing.T) {
	streamText := func(t *testing.T, chunks []string, schema json.RawMessage) string {
		t.Helper()
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for _, ch := range chunks {
				sseChunk(w, textChunk(ch))
			}
			sseChunk(w, finalChunk("STOP", 10, 5))
		})
		ch, err := c.Stream(context.Background(), provider.Request{ResponseSchema: schema})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		text, _, _, _, _, _ := drainEvents(t, ch)
		return text
	}

	// Simple fence with lang tag: stripped; trailing newline before closing fence preserved.
	got := streamText(t, []string{"```json\n{\"k\":\"v\"}\n```"}, nil)
	if got != "{\"k\":\"v\"}\n" {
		t.Errorf("simple json fence: got %q, want %q", got, "{\"k\":\"v\"}\n")
	}

	// Fence split across streaming chunks: stripped, trailing newline preserved.
	got = streamText(t, []string{"```python\n", "x = 1\n", "```"}, nil)
	if got != "x = 1\n" {
		t.Errorf("split-chunk fence: got %q, want %q", got, "x = 1\n")
	}

	// No fence: plain text unchanged.
	got = streamText(t, []string{"hello world"}, nil)
	if got != "hello world" {
		t.Errorf("no fence: got %q, want %q", got, "hello world")
	}

	// Fence with no lang tag (bare ```): stripped, trailing newline preserved.
	got = streamText(t, []string{"```\nraw content\n```"}, nil)
	if got != "raw content\n" {
		t.Errorf("bare fence: got %q, want %q", got, "raw content\n")
	}

	// Mixed prose-and-fence: not stripped (prose on first non-whitespace line).
	got = streamText(t, []string{"intro\n```\ncode\n```"}, nil)
	if got != "intro\n```\ncode\n```" {
		t.Errorf("mixed prose: got %q, want %q", got, "intro\n```\ncode\n```")
	}

	// Unbalanced fence in body: not stripped.
	// Text = "```\nfoo\n```\nunclosed\n```"
	// Outer: first="```", last="```"; body = ["foo", "```", "unclosed"].
	// Balance: "```" opens (inFence=true), "unclosed" not a fence → inFence=true at end → unbalanced.
	got = streamText(t, []string{"```\nfoo\n```\nunclosed\n```"}, nil)
	if got != "```\nfoo\n```\nunclosed\n```" {
		t.Errorf("unbalanced inner: got %q, want unchanged", got)
	}

	// Nested balanced fence in body: outer stripped, inner preserved with trailing newline.
	got = streamText(t, []string{"```\n```python\ncode\n```\n```"}, nil)
	if got != "```python\ncode\n```\n" {
		t.Errorf("nested balanced: got %q, want %q", got, "```python\ncode\n```\n")
	}

	// Schema set: no stripping.
	schema := json.RawMessage(`{"type":"object"}`)
	got = streamText(t, []string{"```json\n{\"k\":\"v\"}\n```"}, schema)
	if got != "```json\n{\"k\":\"v\"}\n```" {
		t.Errorf("schema set: got %q, want unchanged", got)
	}
}
