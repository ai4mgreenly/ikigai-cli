package startup

import (
	"strings"
	"testing"
)

// R-YL2Y-7HXQ: missing ANTHROPIC_API_KEY is a fatal startup error
// for the MVP (Anthropic-only) provider surface.
func TestR_YL2Y_7HXQ_MissingAnthropicKeyIsFatalStartupError(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"unset", ""},
		{"empty", ""},
		{"whitespace", "   \t\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := func(k string) string {
				if k == AnthropicKeyEnv {
					return tc.val
				}
				return ""
			}
			err := requireAnthropicKey(env)
			if err == nil {
				t.Fatalf("expected error when %s is %q, got nil", AnthropicKeyEnv, tc.val)
			}
			if !strings.Contains(err.Error(), AnthropicKeyEnv) {
				t.Fatalf("error must name the missing env var %q; got %q", AnthropicKeyEnv, err.Error())
			}
		})
	}

	t.Run("present", func(t *testing.T) {
		env := func(k string) string {
			if k == AnthropicKeyEnv {
				return "sk-ant-test"
			}
			return ""
		}
		if err := requireAnthropicKey(env); err != nil {
			t.Fatalf("unexpected error with key set: %v", err)
		}
	})
}
