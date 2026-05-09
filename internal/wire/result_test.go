package wire_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// R-13ZB-EZZK: the result event has shape
// {"type":"result","structured_output":<json-value>,"is_error":<bool>}.
// structured_output is required; is_error flags iteration-level
// failure. Optional fields (num_turns, duration_ms, total_cost_usd,
// usage) are out of MVP scope and must not leak into the encoded
// object as zero-valued required fields.
func TestR_13ZB_EZZK_ResultEventShape(t *testing.T) {
	ev, err := wire.NewResultEvent(map[string]string{"status": "CONTINUE"}, false)
	if err != nil {
		t.Fatalf("NewResultEvent: %v", err)
	}
	if ev.Type != "result" {
		t.Errorf("Type = %q, want %q", ev.Type, "result")
	}

	var buf bytes.Buffer
	if err := wire.Encode(&buf, ev); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	line := strings.TrimSuffix(buf.String(), "\n")

	// Decode generically and assert exactly the three MVP keys.
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["type"] != "result" {
		t.Errorf("type = %v, want %q", got["type"], "result")
	}
	so, ok := got["structured_output"].(map[string]any)
	if !ok {
		t.Fatalf("structured_output not an object: %v", got["structured_output"])
	}
	if so["status"] != "CONTINUE" {
		t.Errorf("structured_output.status = %v, want %q", so["status"], "CONTINUE")
	}
	if got["is_error"] != false {
		t.Errorf("is_error = %v, want false", got["is_error"])
	}

	wantKeys := map[string]bool{"type": true, "structured_output": true, "is_error": true}
	for k := range got {
		if !wantKeys[k] {
			t.Errorf("unexpected key in MVP result event: %q", k)
		}
	}
	for k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("missing required key: %q", k)
		}
	}

	// is_error: true must round-trip with a structured_output value
	// that the schema would reject, per R-1OPL-X3LD's pattern (the
	// value here doesn't matter — we just assert is_error encodes).
	errEv, err := wire.NewResultEvent(map[string]string{"oops": "schema-fail"}, true)
	if err != nil {
		t.Fatalf("NewResultEvent: %v", err)
	}
	buf.Reset()
	if err := wire.Encode(&buf, errEv); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var got2 map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got2["is_error"] != true {
		t.Errorf("is_error = %v, want true", got2["is_error"])
	}

	// structured_output accepts arbitrary JSON values, not just objects.
	scalarEv, err := wire.NewResultEvent("DONE", false)
	if err != nil {
		t.Fatalf("NewResultEvent scalar: %v", err)
	}
	buf.Reset()
	if err := wire.Encode(&buf, scalarEv); err != nil {
		t.Fatalf("Encode scalar: %v", err)
	}
	var got3 map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got3); err != nil {
		t.Fatalf("decode scalar: %v", err)
	}
	if got3["structured_output"] != "DONE" {
		t.Errorf("structured_output scalar = %v, want %q", got3["structured_output"], "DONE")
	}
}
