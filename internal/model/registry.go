// R-YRPM-NUDF: model and effort validation is data-driven from a
// per-(provider, model) registry that lives in code as a const-style
// table. Adding a model means editing this table and shipping a new
// binary; no runtime config, no on-disk loading.
//
// R-ZCFX-5XZ8: a --model value that parses to a known provider but is
// not in the registry is rejected at startup with an error listing
// the supported models for that provider.
//
// R-MPR7-P0A4: MVP Anthropic backend supports two models:
//   - claude-haiku-4-5: no --effort accepted
//   - claude-sonnet-4-6: accepts low|medium|high|xhigh|max
// The other providers' rows are intentionally absent in v1; the table
// shape is what makes adding them later a data edit, not an
// architectural change.
package model

import (
	"fmt"
	"sort"
	"strings"
)

// efforts holds the legal --effort vocabulary for a given model. A
// nil slice means the model takes no --effort argument at all (per
// R-MPR7-P0A4 for Haiku 4.5). A non-nil slice lists the accepted
// values; the per-flag effort-validation pass (R-ZX67-O1L1) reads it.
type modelSpec struct {
	efforts []string
}

// registry: provider -> bare model ID -> spec. The keys carry any
// suffix that distinguishes context variants (e.g. "[1m]"); the
// resolved bare ID is matched verbatim.
// R-MPR7-P0A4: Anthropic MVP models.
var registry = map[Provider]map[string]modelSpec{
	ProviderAnthropic: {
		"claude-haiku-4-5":  {efforts: nil},
		"claude-sonnet-4-6": {efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	},
}

// Validate checks that the resolved (provider, bare ID) is present in
// the registry. On miss it returns an error listing the supported
// models for that provider, sorted for stable output.
func Validate(r Resolved) error {
	models, ok := registry[r.Provider]
	if !ok || len(models) == 0 {
		return fmt.Errorf("--model %q: provider %q has no supported models in this build", r.BareID, r.Provider)
	}
	if _, ok := models[r.BareID]; ok {
		return nil
	}
	return fmt.Errorf("--model %q is not supported (provider %s); supported models: %s",
		r.BareID, r.Provider, strings.Join(supportedModels(models), ", "))
}

// ValidateEffort checks that the supplied --effort value is legal for
// the resolved model per the registry. R-31CY-UXSX: when the model's
// efforts slice is nil, the model takes no --effort at all and any
// non-empty value is rejected with an error naming the supported value
// as "(none)". When the slice is non-nil, an empty effort is accepted
// (caller is expected to fall back to a model-specific default) and a
// non-empty effort must be a member of the slice.
func ValidateEffort(r Resolved, effort string) error {
	models, ok := registry[r.Provider]
	if !ok {
		return fmt.Errorf("--effort %q: provider %q has no supported models in this build", effort, r.Provider)
	}
	spec, ok := models[r.BareID]
	if !ok {
		return fmt.Errorf("--effort %q: model %q is not in the registry", effort, r.BareID)
	}
	if spec.efforts == nil {
		if effort == "" {
			return nil
		}
		return fmt.Errorf("--effort %q is not supported for model %q; supported values: (none)", effort, r.BareID)
	}
	if effort == "" {
		return nil
	}
	for _, e := range spec.efforts {
		if e == effort {
			return nil
		}
	}
	return fmt.Errorf("--effort %q is not supported for model %q; supported values: %s",
		effort, r.BareID, strings.Join(spec.efforts, ", "))
}

func supportedModels(m map[string]modelSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
