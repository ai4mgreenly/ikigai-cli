package model

import (
	"strings"
	"testing"
)

// R-YRPM-NUDF: registry is a const-style data table; MVP entries for
// the Anthropic backend (R-MPR7-P0A4) must be present and accepted by
// Validate without per-call configuration.
func TestR_YRPM_NUDF_RegistryHoldsHaikuMVPEntry(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-haiku-4-5"}
	if err := Validate(r); err != nil {
		t.Fatalf("Validate(haiku MVP) = %v, want nil", err)
	}

	models, ok := registry[ProviderAnthropic]
	if !ok {
		t.Fatalf("registry missing Anthropic provider row")
	}
	if _, ok := models["claude-haiku-4-5"]; !ok {
		t.Errorf("Anthropic registry missing claude-haiku-4-5; have %v", supportedModels(models))
	}
}

// R-ZCFX-5XZ8: a --model that parses to a known provider but is not
// in the registry is rejected at startup; the error must list the
// supported models for that provider so the user can recover.
func TestR_ZCFX_5XZ8_UnknownModelRejectedWithSupportedList(t *testing.T) {
	cases := []struct {
		name string
		in   Resolved
	}{
		{"opus not in MVP", Resolved{ProviderAnthropic, "claude-opus-4-7"}},
		{"haiku 1m variant not in MVP", Resolved{ProviderAnthropic, "claude-haiku-4-5[1m]"}},
		{"openai deferred", Resolved{ProviderOpenAI, "gpt-5.4"}},
		{"google deferred", Resolved{ProviderGoogle, "gemini-3-pro-preview"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.in)
			if err == nil {
				t.Fatalf("Validate(%+v) returned nil, want rejection", tc.in)
			}
			if tc.in.Provider == ProviderAnthropic {
				if !strings.Contains(err.Error(), "claude-haiku-4-5") {
					t.Errorf("Anthropic rejection %q must list claude-haiku-4-5", err.Error())
				}
				if !strings.Contains(err.Error(), "claude-sonnet-4-6") {
					t.Errorf("Anthropic rejection %q must list claude-sonnet-4-6", err.Error())
				}
			}
		})
	}
}

// R-MPR7-P0A4: Haiku 4.5 takes no --effort argument; supplying one
// must be rejected with an error naming the supported value as
// "(none)". An empty effort is the MVP default and must be accepted.
func TestR_MPR7_P0A4_HaikuRejectsEffort(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-haiku-4-5"}
	for _, effort := range []string{"low", "medium", "high", "minimal", "thinking"} {
		t.Run(effort, func(t *testing.T) {
			err := ValidateEffort(r, effort)
			if err == nil {
				t.Fatalf("ValidateEffort(haiku, %q) = nil, want rejection", effort)
			}
			if !strings.Contains(err.Error(), "(none)") {
				t.Errorf("error %q must name supported value as (none)", err.Error())
			}
		})
	}
}

func TestR_MPR7_P0A4_HaikuAcceptsNoEffort(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-haiku-4-5"}
	if err := ValidateEffort(r, ""); err != nil {
		t.Fatalf("ValidateEffort(haiku, \"\") = %v, want nil", err)
	}
}

// R-MPR7-P0A4: Sonnet 4.6 accepts effort values low|medium|high|xhigh|max
// and rejects any other non-empty value.
func TestR_MPR7_P0A4_SonnetAcceptsValidEfforts(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-sonnet-4-6"}
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			if err := ValidateEffort(r, effort); err != nil {
				t.Errorf("ValidateEffort(sonnet, %q) = %v, want nil", effort, err)
			}
		})
	}
}

func TestR_MPR7_P0A4_SonnetAcceptsNoEffort(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-sonnet-4-6"}
	if err := ValidateEffort(r, ""); err != nil {
		t.Fatalf("ValidateEffort(sonnet, \"\") = %v, want nil", err)
	}
}

func TestR_MPR7_P0A4_SonnetRejectsUnknownEffort(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-sonnet-4-6"}
	for _, effort := range []string{"minimal", "thinking", "turbo", "fast"} {
		t.Run(effort, func(t *testing.T) {
			err := ValidateEffort(r, effort)
			if err == nil {
				t.Fatalf("ValidateEffort(sonnet, %q) = nil, want rejection", effort)
			}
			if !strings.Contains(err.Error(), "supported values:") {
				t.Errorf("error %q must list values via \"supported values:\"", err.Error())
			}
		})
	}
}

// R-MPR7-P0A4: both MVP models must be present in the registry so
// Validate accepts them without per-call configuration.
func TestR_MPR7_P0A4_BothModelsInRegistry(t *testing.T) {
	models, ok := registry[ProviderAnthropic]
	if !ok {
		t.Fatal("registry missing Anthropic provider row")
	}
	for _, id := range []string{"claude-haiku-4-5", "claude-sonnet-4-6"} {
		if _, ok := models[id]; !ok {
			t.Errorf("Anthropic registry missing %q; have %v", id, supportedModels(models))
		}
		r := Resolved{Provider: ProviderAnthropic, BareID: id}
		if err := Validate(r); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", id, err)
		}
	}
}

// R-ZX67-O1L1: a --effort value that is not legal for the selected
// model is rejected at startup with an error listing the legal values
// for that model. The legal-values listing is the user-facing recovery
// signal; this test pins the "supported values:" reporting contract
// that all models with a non-empty effort vocabulary must satisfy.
func TestR_ZX67_O1L1_IllegalEffortListsSupportedValues(t *testing.T) {
	r := Resolved{Provider: ProviderAnthropic, BareID: "claude-haiku-4-5"}
	err := ValidateEffort(r, "high")
	if err == nil {
		t.Fatal("expected error for illegal effort")
	}
	msg := err.Error()
	if !strings.Contains(msg, "supported values:") {
		t.Errorf("error must list legal values via \"supported values:\" prefix; got: %q", msg)
	}
	if !strings.Contains(msg, "claude-haiku-4-5") {
		t.Errorf("error must name the rejected model so the operator knows which vocabulary applies; got: %q", msg)
	}
	if !strings.Contains(msg, "high") {
		t.Errorf("error must echo the rejected effort value; got: %q", msg)
	}
}

// R-Y23Q-MNSU: provider is inferred from the bare API ID's prefix —
// claude-* → Anthropic, gpt-* → OpenAI, gemini-* → Google. Pin the
// three rules with values that aren't (and won't become) registry
// entries, so this test guards the prefix mapping itself.
func TestR_Y23Q_MNSU_ProviderInferredFromPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want Provider
	}{
		{"claude-future-x", ProviderAnthropic},
		{"gpt-future-x", ProviderOpenAI},
		{"gemini-future-x", ProviderGoogle},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Resolve(tc.in)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tc.in, err)
			}
			if got.Provider != tc.want {
				t.Errorf("Resolve(%q).Provider = %q, want %q", tc.in, got.Provider, tc.want)
			}
		})
	}
}
