package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	"github.com/ai4mgreenly/ikigai-cli/internal/schema"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// fakeClient is a provider.Client whose Stream replays a fixed
// sequence of events. The channel is closed after the last event so
// drainTurn's range terminates.
type fakeClient struct {
	events []provider.Event
	err    error
}

func (f *fakeClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan provider.Event, len(f.events))
	for _, ev := range f.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// R-VJBZ-S578: a single iteration terminates with exactly one result
// event whose structured_output validates against the supplied
// --json-schema. The simplest slice: end_turn with assistant text that
// is the literal ralph-loops control object.
func TestR_VJBZ_S578_IterationEmitsOneResultMatchingSchema(t *testing.T) {
	const ralphSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["DONE", "CONTINUE"]}
		},
		"required": ["status"],
		"additionalProperties": false
	}`
	sch, err := schema.Parse([]byte(ralphSchema))
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}

	client := &fakeClient{events: []provider.Event{
		provider.EventTextDelta{Text: `{"status":"DONE"}`},
		provider.EventDone{StopReason: "end_turn"},
	}}

	var out bytes.Buffer
	sess := wire.NewSession(&out)

	if err := Run(context.Background(), client, sess, provider.Request{}, sch, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := splitLines(out.String())
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 events (assistant + result), got %d: %q", len(lines), out.String())
	}

	var assistant struct {
		Type    string `json:"type"`
		Message struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &assistant); err != nil {
		t.Fatalf("unmarshal assistant: %v", err)
	}
	if assistant.Type != "assistant" {
		t.Fatalf("first event type = %q, want assistant", assistant.Type)
	}
	if len(assistant.Message.Content) != 1 || assistant.Message.Content[0]["type"] != "text" {
		t.Fatalf("assistant content = %+v, want one text block", assistant.Message.Content)
	}

	var result struct {
		Type             string          `json:"type"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		IsError          bool            `json:"is_error"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Type != "result" {
		t.Fatalf("second event type = %q, want result", result.Type)
	}
	if result.IsError {
		t.Fatalf("is_error = true, want false; structured_output=%s", result.StructuredOutput)
	}

	var got any
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatalf("unmarshal structured_output: %v", err)
	}
	if err := sch.Validate(got); err != nil {
		t.Fatalf("emitted structured_output failed schema: %v", err)
	}
	gotMap, ok := got.(map[string]any)
	if !ok || gotMap["status"] != "DONE" {
		t.Fatalf("structured_output = %v, want {status:DONE}", got)
	}

	if !sess.Finished() {
		t.Fatalf("session not finished after Run")
	}
}

// sequenceClient is a provider.Client whose Stream replays a different
// fixed sequence on each call, in order. It is used to simulate a
// model that returns invalid structured_output on early attempts and a
// schema-conforming object on a later one.
type sequenceClient struct {
	sequences [][]provider.Event
	calls     int
}

func (s *sequenceClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	if s.calls >= len(s.sequences) {
		return nil, &provider.Error{Kind: provider.ErrUnknown, Msg: "sequenceClient exhausted"}
	}
	evs := s.sequences[s.calls]
	s.calls++
	ch := make(chan provider.Event, len(evs))
	for _, ev := range evs {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// R-WFWM-BKWX: when the model produces output that fails to validate
// against the supplied --json-schema, the agent retries the model up
// to a bounded number of times before surfacing an iteration error.
// On a successful retry the iteration emits exactly one result event;
// every attempt's turn is recorded as its own assistant event.
func TestR_WFWM_BKWX_AgentRetriesOnValidationFailure(t *testing.T) {
	const ralphSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["DONE", "CONTINUE"]}
		},
		"required": ["status"],
		"additionalProperties": false
	}`
	sch, err := schema.Parse([]byte(ralphSchema))
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}

	// Two failing attempts (malformed JSON, then schema-violating
	// JSON) followed by a passing one. Verifies bounded retry.
	t.Run("eventual_success", func(t *testing.T) {
		client := &sequenceClient{sequences: [][]provider.Event{
			{
				provider.EventTextDelta{Text: `not json at all`},
				provider.EventDone{StopReason: "end_turn"},
			},
			{
				provider.EventTextDelta{Text: `{"status":"NOPE"}`},
				provider.EventDone{StopReason: "end_turn"},
			},
			{
				provider.EventTextDelta{Text: `{"status":"DONE"}`},
				provider.EventDone{StopReason: "end_turn"},
			},
		}}

		var out bytes.Buffer
		sess := wire.NewSession(&out)

		if err := Run(context.Background(), client, sess, provider.Request{}, sch, nil); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if client.calls != 3 {
			t.Fatalf("client.calls = %d, want 3", client.calls)
		}

		lines := splitLines(out.String())
		if len(lines) != 4 {
			t.Fatalf("expected 4 events (3 assistants + result), got %d: %q", len(lines), out.String())
		}
		for i := 0; i < 3; i++ {
			var ev struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(lines[i]), &ev); err != nil {
				t.Fatalf("unmarshal line %d: %v", i, err)
			}
			if ev.Type != "assistant" {
				t.Fatalf("line %d type = %q, want assistant", i, ev.Type)
			}
		}
		var result struct {
			Type             string          `json:"type"`
			StructuredOutput json.RawMessage `json:"structured_output"`
			IsError          bool            `json:"is_error"`
		}
		if err := json.Unmarshal([]byte(lines[3]), &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if result.Type != "result" {
			t.Fatalf("last event type = %q, want result", result.Type)
		}
		if result.IsError {
			t.Fatalf("is_error = true, want false; structured_output=%s", result.StructuredOutput)
		}
		var got map[string]any
		if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
			t.Fatalf("unmarshal structured_output: %v", err)
		}
		if got["status"] != "DONE" {
			t.Fatalf("structured_output = %v, want {status:DONE}", got)
		}
	})

	// All maxStructuredAttempts attempts fail validation. The agent
	// emits one assistant per attempt and a final is_error result.
	t.Run("exhausted_retries", func(t *testing.T) {
		bad := []provider.Event{
			provider.EventTextDelta{Text: `{"status":"NOPE"}`},
			provider.EventDone{StopReason: "end_turn"},
		}
		client := &sequenceClient{sequences: [][]provider.Event{bad, bad, bad}}

		var out bytes.Buffer
		sess := wire.NewSession(&out)

		if err := Run(context.Background(), client, sess, provider.Request{}, sch, nil); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if client.calls != maxStructuredAttempts {
			t.Fatalf("client.calls = %d, want %d", client.calls, maxStructuredAttempts)
		}

		lines := splitLines(out.String())
		if len(lines) != maxStructuredAttempts+1 {
			t.Fatalf("expected %d events, got %d: %q", maxStructuredAttempts+1, len(lines), out.String())
		}
		var result struct {
			Type             string          `json:"type"`
			StructuredOutput json.RawMessage `json:"structured_output"`
			IsError          bool            `json:"is_error"`
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if result.Type != "result" || !result.IsError {
			t.Fatalf("last event = %+v, want result with is_error=true", result)
		}
		var payload map[string]string
		if err := json.Unmarshal(result.StructuredOutput, &payload); err != nil {
			t.Fatalf("unmarshal structured_output: %v", err)
		}
		if !strings.Contains(payload["error"], "after 3 attempts") {
			t.Fatalf("error message = %q, want it to mention attempts count", payload["error"])
		}
	})
}

// R-8PF6-I8FP: ikigai-cli sends a non-empty system prompt on every
// provider request so that the model operates as an agent rather than
// a plain chatbot.
func TestR_8PF6_I8FP_FramingPromptIsNonEmpty(t *testing.T) {
	if FramingPrompt == "" {
		t.Fatal("FramingPrompt must be non-empty (R-8PF6-I8FP): the system prompt is required to orient the model as an agent")
	}
}

// R-8293-8LCI: when an assistant turn ends with tool_use, the agent
// dispatches every tool_use block, emits a user event with tool_results,
// appends both turns to history, and re-invokes the provider. The
// iteration ends when the model returns a non-tool stop reason.
func TestR_8293_8LCI_ToolRoundTripDispatchesToolsAndContinues(t *testing.T) {
	// Write a known file the Read tool can consume.
	tmp := t.TempDir()
	filePath := tmp + "/hello.txt"
	if err := os.WriteFile(filePath, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	toolInput, err := json.Marshal(map[string]string{"file_path": filePath})
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}

	// First provider call: the model asks to read the temp file.
	// Second provider call: the model returns the final structured output.
	client := &sequenceClient{sequences: [][]provider.Event{
		{
			provider.EventToolUse{ID: "toolu_01", Name: "Read", Input: toolInput},
			provider.EventDone{StopReason: "tool_use"},
		},
		{
			provider.EventTextDelta{Text: `{"status":"DONE"}`},
			provider.EventDone{StopReason: "end_turn"},
		},
	}}

	const ralphSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["DONE", "CONTINUE"]}
		},
		"required": ["status"],
		"additionalProperties": false
	}`
	sch, err := schema.Parse([]byte(ralphSchema))
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}

	var out bytes.Buffer
	sess := wire.NewSession(&out)
	if err := Run(context.Background(), client, sess, provider.Request{}, sch, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("client.calls = %d, want 2 (one tool_use + one end_turn)", client.calls)
	}

	lines := splitLines(out.String())
	// Expected: assistant(tool_use), user(tool_result), assistant(text), result
	if len(lines) != 4 {
		t.Fatalf("expected 4 events, got %d: %q", len(lines), out.String())
	}

	// Line 0: assistant with tool_use block.
	var assistantEv struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &assistantEv); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if assistantEv.Type != "assistant" {
		t.Fatalf("line 0 type = %q, want assistant", assistantEv.Type)
	}
	if len(assistantEv.Message.Content) != 1 || assistantEv.Message.Content[0].Type != "tool_use" {
		t.Fatalf("line 0 content = %+v, want one tool_use block", assistantEv.Message.Content)
	}
	toolUseID := assistantEv.Message.Content[0].ID
	if toolUseID != "toolu_01" {
		t.Fatalf("tool_use id = %q, want toolu_01", toolUseID)
	}

	// Line 1: user with tool_result block correlated by id.
	var userEv struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				IsError   bool   `json:"is_error"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &userEv); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if userEv.Type != "user" {
		t.Fatalf("line 1 type = %q, want user", userEv.Type)
	}
	if len(userEv.Message.Content) != 1 || userEv.Message.Content[0].Type != "tool_result" {
		t.Fatalf("line 1 content = %+v, want one tool_result block", userEv.Message.Content)
	}
	if userEv.Message.Content[0].ToolUseID != toolUseID {
		t.Fatalf("tool_result tool_use_id = %q, want %q", userEv.Message.Content[0].ToolUseID, toolUseID)
	}
	if userEv.Message.Content[0].IsError {
		t.Fatalf("tool_result is_error = true; Read should have succeeded")
	}

	// Line 2: second assistant with text.
	var assistantEv2 struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &assistantEv2); err != nil {
		t.Fatalf("unmarshal line 2: %v", err)
	}
	if assistantEv2.Type != "assistant" {
		t.Fatalf("line 2 type = %q, want assistant", assistantEv2.Type)
	}

	// Line 3: result with schema-conforming structured_output.
	var resultEv struct {
		Type             string          `json:"type"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		IsError          bool            `json:"is_error"`
	}
	if err := json.Unmarshal([]byte(lines[3]), &resultEv); err != nil {
		t.Fatalf("unmarshal line 3: %v", err)
	}
	if resultEv.Type != "result" {
		t.Fatalf("line 3 type = %q, want result", resultEv.Type)
	}
	if resultEv.IsError {
		t.Fatalf("result is_error = true; structured_output = %s", resultEv.StructuredOutput)
	}
	var payload map[string]any
	if err := json.Unmarshal(resultEv.StructuredOutput, &payload); err != nil {
		t.Fatalf("unmarshal structured_output: %v", err)
	}
	if payload["status"] != "DONE" {
		t.Fatalf("structured_output = %v, want {status:DONE}", payload)
	}

	if !sess.Finished() {
		t.Fatalf("session not finished after Run")
	}
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
