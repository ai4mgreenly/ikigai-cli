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

// R-0W9B-7E8I: OpenAI credential lives in OPENAI_API_KEY.
const OpenAIKeyEnv = "OPENAI_API_KEY"

// R-KIGL-EK0W: Google credential lives in GOOGLE_API_KEY.
const GoogleKeyEnv = "GOOGLE_API_KEY"

// R-857T-2AX4: RequireCredential reads only the env var for the selected
// provider. providerName must be one of "anthropic", "openai", "google". The error
// message names the missing variable for the selected provider — never a
// different provider's variable.
func RequireCredential(providerName string) error {
	return requireCredential(providerName, os.Getenv)
}

func requireCredential(providerName string, getenv func(string) string) error {
	switch providerName {
	case "anthropic":
		return requireAnthropicKey(getenv)
	case "openai":
		return requireOpenAIKey(getenv)
	case "google":
		return requireGoogleKey(getenv)
	default:
		return errors.New("unknown provider " + providerName + ": no credential check defined")
	}
}

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

// RequireOpenAIKey returns a non-nil error naming the missing
// environment variable when OPENAI_API_KEY is unset or empty.
// R-0W9B-7E8I: authentication uses OPENAI_API_KEY as a bearer
// credential; a missing key is a fatal startup error for the
// OpenAI backend.
func RequireOpenAIKey() error {
	return requireOpenAIKey(os.Getenv)
}

func requireOpenAIKey(getenv func(string) string) error {
	if strings.TrimSpace(getenv(OpenAIKeyEnv)) == "" {
		return errors.New(OpenAIKeyEnv + " is not set: ikigai-cli requires an OpenAI API key to start")
	}
	return nil
}

// RequireGoogleKey returns a non-nil error naming the missing environment
// variable when GOOGLE_API_KEY is unset or empty.
// R-KIGL-EK0W: authentication uses GOOGLE_API_KEY; a missing key is a fatal
// startup error for the Google backend.
func RequireGoogleKey() error {
	return requireGoogleKey(os.Getenv)
}

func requireGoogleKey(getenv func(string) string) error {
	if strings.TrimSpace(getenv(GoogleKeyEnv)) == "" {
		return errors.New(GoogleKeyEnv + " is not set: ikigai-cli requires a Google API key to start")
	}
	return nil
}
