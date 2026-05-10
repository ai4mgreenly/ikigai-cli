package driver_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai4mgreenly/ikigai-cli/internal/driver"
	"github.com/ai4mgreenly/ikigai-cli/internal/provider"
	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// TestR_4WFF_WBL4_ThinkingBlocksForwardedToStdout verifies that an
// Anthropic-style EventThinking on the provider stream is emitted on
// stdout as a wire thinking block of shape
// {"type":"thinking","thinking":"<string>"} per wire-format R-SA9P-R1H4.
//
// R-4WFF-WBL4: providers.md "Anthropic's `thinking` content blocks
// (when emitted by the model in adaptive-thinking mode) are forwarded
// to stdout as `thinking` blocks per wire-format.md R-SA9P-R1H4."
func TestR_4WFF_WBL4_ThinkingBlocksForwardedToStdout(t *testing.T) {
	events := make(chan provider.Event, 4)
	events <- provider.EventThinking{Text: "weighing options", Signature: "sig-1"}
	events <- provider.EventTextDelta{Text: "hello"}
	events <- provider.EventDone{StopReason: "end_turn"}
	close(events)

	var out bytes.Buffer
	sess := wire.NewSession(&out)
	stop, err := driver.EmitAssistantTurn(sess, events)
	if err != nil {
		t.Fatalf("EmitAssistantTurn: %v", err)
	}
	if stop != "end_turn" {
		t.Errorf("stop reason = %q, want %q", stop, "end_turn")
	}

	line := strings.TrimRight(out.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Fatalf("expected exactly one line, got %q", out.String())
	}

	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("decode emitted event: %v\nline: %s", err, line)
	}
	if ev.Type != "assistant" || ev.Message.Role != "assistant" {
		t.Errorf("event shape = %+v, want type+role assistant", ev)
	}
	if len(ev.Message.Content) != 2 {
		t.Fatalf("expected 2 blocks (thinking, text), got %d: %v", len(ev.Message.Content), ev.Message.Content)
	}

	first := ev.Message.Content[0]
	if first["type"] != "thinking" {
		t.Errorf("first block type = %v, want thinking", first["type"])
	}
	if first["thinking"] != "weighing options" {
		t.Errorf("thinking text = %v, want %q", first["thinking"], "weighing options")
	}
	if _, hasSig := first["signature"]; hasSig {
		t.Errorf("wire thinking block must not carry signature; got %v", first)
	}

	second := ev.Message.Content[1]
	if second["type"] != "text" || second["text"] != "hello" {
		t.Errorf("second block = %v, want text/hello", second)
	}
}

// Drives an empty stream cleanly: the driver still emits one assistant
// event (with zero blocks) so the iteration's invariants hold.
func TestEmitAssistantTurn_EmptyStream(t *testing.T) {
	events := make(chan provider.Event)
	close(events)
	var out bytes.Buffer
	sess := wire.NewSession(&out)
	stop, err := driver.EmitAssistantTurn(sess, events)
	if err != nil {
		t.Fatalf("EmitAssistantTurn: %v", err)
	}
	if stop != "" {
		t.Errorf("stop = %q, want empty", stop)
	}
	line := strings.TrimRight(out.String(), "\n")
	if !strings.Contains(line, `"content":[]`) {
		t.Errorf("expected empty content array, got %s", line)
	}
}

// TestR_ZRRF_LGW1_ToolCallAndResultOnStdout pins the spec invariant
// that tool-call and tool-result turns appear on stdout in Claude's
// stream-json shape regardless of how the underlying provider
// represented them on the wire: an assistant event containing a
// tool_use block followed by a user event containing a tool_result
// block whose tool_use_id matches.
//
// R-ZRRF-LGW1: tool-call and tool-result turns appear on stdout in
// Claude's stream-json shape (`assistant` events containing
// `tool_use` blocks, `user` events containing `tool_result` blocks),
// regardless of how the underlying provider represented them on the
// wire.
func TestR_ZRRF_LGW1_ToolCallAndResultOnStdout(t *testing.T) {
	events := make(chan provider.Event, 3)
	events <- provider.EventToolUse{ID: "u1", Name: "read", Input: json.RawMessage(`{"path":"x"}`)}
	events <- provider.EventDone{StopReason: "tool_use"}
	close(events)

	var out bytes.Buffer
	sess := wire.NewSession(&out)
	if _, err := driver.EmitAssistantTurn(sess, events); err != nil {
		t.Fatalf("EmitAssistantTurn: %v", err)
	}

	res, err := wire.NewToolResultBlock("u1", false, "file contents")
	if err != nil {
		t.Fatalf("NewToolResultBlock: %v", err)
	}
	if err := sess.EmitUser(wire.NewUserEvent(res)); err != nil {
		t.Fatalf("EmitUser: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (assistant, user), got %d:\n%s", len(lines), out.String())
	}

	var asst struct {
		Type    string `json:"type"`
		Message struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &asst); err != nil {
		t.Fatalf("decode assistant line: %v\n%s", err, lines[0])
	}
	if asst.Type != "assistant" || asst.Message.Role != "assistant" {
		t.Errorf("first line shape = %+v, want assistant/assistant", asst)
	}
	if len(asst.Message.Content) != 1 || asst.Message.Content[0]["type"] != "tool_use" {
		t.Fatalf("first line content = %v, want one tool_use block", asst.Message.Content)
	}
	tu := asst.Message.Content[0]
	if tu["id"] != "u1" || tu["name"] != "read" {
		t.Errorf("tool_use block = %v, want id=u1 name=read", tu)
	}

	var usr struct {
		Type    string `json:"type"`
		Message struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &usr); err != nil {
		t.Fatalf("decode user line: %v\n%s", err, lines[1])
	}
	if usr.Type != "user" || usr.Message.Role != "user" {
		t.Errorf("second line shape = %+v, want user/user", usr)
	}
	if len(usr.Message.Content) != 1 || usr.Message.Content[0]["type"] != "tool_result" {
		t.Fatalf("second line content = %v, want one tool_result block", usr.Message.Content)
	}
	tr := usr.Message.Content[0]
	if tr["tool_use_id"] != "u1" {
		t.Errorf("tool_result.tool_use_id = %v, want u1", tr["tool_use_id"])
	}
	if tr["is_error"] != false {
		t.Errorf("tool_result.is_error = %v, want false", tr["is_error"])
	}
	if tr["content"] != "file contents" {
		t.Errorf("tool_result.content = %v, want %q", tr["content"], "file contents")
	}
}

// Verifies tool_use events round-trip through the driver into the
// assistant event so wire.Session can track the pending id correctly.
func TestEmitAssistantTurn_ToolUseRoundTrips(t *testing.T) {
	events := make(chan provider.Event, 2)
	events <- provider.EventToolUse{ID: "u1", Name: "read", Input: json.RawMessage(`{"path":"x"}`)}
	events <- provider.EventDone{StopReason: "tool_use"}
	close(events)

	var out bytes.Buffer
	sess := wire.NewSession(&out)
	if _, err := driver.EmitAssistantTurn(sess, events); err != nil {
		t.Fatalf("EmitAssistantTurn: %v", err)
	}
	pending := sess.PendingToolUseIDs()
	if len(pending) != 1 || pending[0] != "u1" {
		t.Errorf("pending = %v, want [u1]", pending)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"input":{"path":"x"}`)) {
		t.Errorf("tool_use input not forwarded byte-stable: %s", out.String())
	}
}
