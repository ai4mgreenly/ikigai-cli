package wire_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// R-YLQR-3Z46: every `user` event has shape
// {"type":"user","message":{"role":"user","content":[<blocks>]}}.
// The constructor must fix message.role to "user" and content must
// always serialize as a JSON array (zero or more blocks), never null.
func TestR_YLQR_3Z46_UserEventShape(t *testing.T) {
	cases := []struct {
		name    string
		content []any
	}{
		{name: "empty", content: nil},
		{
			name: "two_blocks",
			content: []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "text", "text": "world"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := wire.NewUserEvent(tc.content...)

			var buf bytes.Buffer
			if err := wire.Encode(&buf, ev); err != nil {
				t.Fatalf("encode: %v", err)
			}
			line := strings.TrimSuffix(buf.String(), "\n")

			// Empty content must be `[]`, never `null`.
			if tc.content == nil && !strings.Contains(line, `"content":[]`) {
				t.Errorf("empty content not serialized as []: %s", line)
			}

			var got map[string]any
			if err := json.Unmarshal([]byte(line), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if got["type"] != "user" {
				t.Errorf("type = %v, want \"user\"", got["type"])
			}

			msg, ok := got["message"].(map[string]any)
			if !ok {
				t.Fatalf("message missing or not object: %v", got["message"])
			}
			if msg["role"] != "user" {
				t.Errorf("message.role = %v, want \"user\"", msg["role"])
			}
			content, ok := msg["content"].([]any)
			if !ok {
				t.Fatalf("message.content missing or not array: %v", msg["content"])
			}
			if len(content) != len(tc.content) {
				t.Errorf("len(content) = %d, want %d", len(content), len(tc.content))
			}

			// Top-level must contain only {type, message}.
			for k := range got {
				if k != "type" && k != "message" {
					t.Errorf("unexpected top-level key %q", k)
				}
			}
			// message must contain only {role, content}.
			for k := range msg {
				if k != "role" && k != "content" {
					t.Errorf("unexpected message key %q", k)
				}
			}
		})
	}
}
