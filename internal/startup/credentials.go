// Package startup performs fatal-startup-error checks for the
// binary before any provider call is attempted.
package startup

import (
	"errors"
	"os"
	"strings"
)

// R-YL2Y-7HXQ: provider credentials are read from environment
// variables only. For the MVP (R-S04B-QD3D) the only provider is
// Anthropic, whose credential lives in ANTHROPIC_API_KEY. Missing
// credentials are a fatal startup error.
const AnthropicKeyEnv = "ANTHROPIC_API_KEY"

// RequireAnthropicKey returns a non-nil error naming the missing
// environment variable when ANTHROPIC_API_KEY is unset or empty.
// The error message is the operator-facing diagnostic; cmd/ writes
// it to stderr before exiting non-zero.
func RequireAnthropicKey() error {
	return requireAnthropicKey(os.Getenv)
}

func requireAnthropicKey(getenv func(string) string) error {
	if strings.TrimSpace(getenv(AnthropicKeyEnv)) == "" {
		return errors.New(AnthropicKeyEnv + " is not set: ikigai-cli requires an Anthropic API key to start")
	}
	return nil
}
