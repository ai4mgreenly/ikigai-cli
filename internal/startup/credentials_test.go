package startup

import (
	"strings"
	"testing"
)

// R-0W9B-7E8I: missing OPENAI_API_KEY is a fatal startup error
// when the OpenAI backend is selected.
func TestR_0W9B_7E8I_MissingOpenAIKeyIsFatalStartupError(t *testing.T) {
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
				if k == OpenAIKeyEnv {
					return tc.val
				}
				return ""
			}
			err := requireOpenAIKey(env)
			if err == nil {
				t.Fatalf("expected error when %s is %q, got nil", OpenAIKeyEnv, tc.val)
			}
			if !strings.Contains(err.Error(), OpenAIKeyEnv) {
				t.Fatalf("error must name the missing env var %q; got %q", OpenAIKeyEnv, err.Error())
			}
		})
	}

	t.Run("present", func(t *testing.T) {
		env := func(k string) string {
			if k == OpenAIKeyEnv {
				return "sk-openai-test"
			}
			return ""
		}
		if err := requireOpenAIKey(env); err != nil {
			t.Fatalf("unexpected error with key set: %v", err)
		}
	})
}

// R-857T-2AX4: provider selection drives credential selection one-to-one.
// Selecting OpenAI reads only OPENAI_API_KEY; selecting Anthropic reads only
// ANTHROPIC_API_KEY. Each provider's absence of the other provider's key is
// not an error, and the error message must name the selected provider's var.
func TestR_857T_2AX4_ProviderSelectionDrivesCredentialSelection(t *testing.T) {
	t.Run("openai_key_present_anthropic_absent_ok", func(t *testing.T) {
		env := func(k string) string {
			if k == OpenAIKeyEnv {
				return "sk-openai-test"
			}
			return "" // ANTHROPIC_API_KEY absent — must not be an error
		}
		if err := requireCredential("openai", env); err != nil {
			t.Errorf("unexpected error when OPENAI_API_KEY set and ANTHROPIC_API_KEY absent: %v", err)
		}
	})

	t.Run("openai_key_absent_errors_naming_openai_var", func(t *testing.T) {
		env := func(k string) string {
			if k == AnthropicKeyEnv {
				return "sk-ant-test" // Anthropic key present but irrelevant
			}
			return ""
		}
		err := requireCredential("openai", env)
		if err == nil {
			t.Fatal("expected error when OPENAI_API_KEY absent, got nil")
		}
		if !strings.Contains(err.Error(), OpenAIKeyEnv) {
			t.Errorf("error must name %q; got %q", OpenAIKeyEnv, err.Error())
		}
		if strings.Contains(err.Error(), AnthropicKeyEnv) {
			t.Errorf("error must not name %q when OpenAI is the selected provider; got %q", AnthropicKeyEnv, err.Error())
		}
	})

	t.Run("anthropic_key_present_openai_absent_ok", func(t *testing.T) {
		env := func(k string) string {
			if k == AnthropicKeyEnv {
				return "sk-ant-test"
			}
			return "" // OPENAI_API_KEY absent — must not be an error
		}
		if err := requireCredential("anthropic", env); err != nil {
			t.Errorf("unexpected error when ANTHROPIC_API_KEY set and OPENAI_API_KEY absent: %v", err)
		}
	})

	t.Run("anthropic_key_absent_errors_naming_anthropic_var", func(t *testing.T) {
		env := func(k string) string {
			if k == OpenAIKeyEnv {
				return "sk-openai-test" // OpenAI key present but irrelevant
			}
			return ""
		}
		err := requireCredential("anthropic", env)
		if err == nil {
			t.Fatal("expected error when ANTHROPIC_API_KEY absent, got nil")
		}
		if !strings.Contains(err.Error(), AnthropicKeyEnv) {
			t.Errorf("error must name %q; got %q", AnthropicKeyEnv, err.Error())
		}
		if strings.Contains(err.Error(), OpenAIKeyEnv) {
			t.Errorf("error must not name %q when Anthropic is the selected provider; got %q", OpenAIKeyEnv, err.Error())
		}
	})
}

// R-KIGL-EK0W: missing GOOGLE_API_KEY is a fatal startup error when the
// Google backend is selected; the key is sent as x-goog-api-key header.
func TestR_KIGL_EK0W_MissingGoogleKeyIsFatalStartupError(t *testing.T) {
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
				if k == GoogleKeyEnv {
					return tc.val
				}
				return ""
			}
			err := requireGoogleKey(env)
			if err == nil {
				t.Fatalf("expected error when %s is %q, got nil", GoogleKeyEnv, tc.val)
			}
			if !strings.Contains(err.Error(), GoogleKeyEnv) {
				t.Fatalf("error must name the missing env var %q; got %q", GoogleKeyEnv, err.Error())
			}
		})
	}

	t.Run("present", func(t *testing.T) {
		env := func(k string) string {
			if k == GoogleKeyEnv {
				return "AIzaSy-test-key"
			}
			return ""
		}
		if err := requireGoogleKey(env); err != nil {
			t.Fatalf("unexpected error with key set: %v", err)
		}
	})

	t.Run("google_key_absent_errors_naming_google_var", func(t *testing.T) {
		env := func(k string) string { return "" }
		err := requireCredential("google", env)
		if err == nil {
			t.Fatal("expected error when GOOGLE_API_KEY absent, got nil")
		}
		if !strings.Contains(err.Error(), GoogleKeyEnv) {
			t.Errorf("error must name %q; got %q", GoogleKeyEnv, err.Error())
		}
	})

	t.Run("google_key_present_ok", func(t *testing.T) {
		env := func(k string) string {
			if k == GoogleKeyEnv {
				return "AIzaSy-test-key"
			}
			return ""
		}
		if err := requireCredential("google", env); err != nil {
			t.Errorf("unexpected error when GOOGLE_API_KEY set: %v", err)
		}
	})
}

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
