package scope_test

// R-XXWU-XUUJ: --effort is passed through in each provider's and
// model's native vocabulary. ikigai-cli must not introduce a
// universal effort scale that gets translated into per-provider
// values. Validation is a per-(provider, model) lookup against the
// registry; there is no normalization layer.
//
// This test enforces the rule statically by scanning non-test source
// for identifiers that would only exist if a translation layer were
// being built (a shared effort-name map, a normalization function,
// an Effort type with translation methods, etc.).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_XXWU_XUUJ_NoEffortNormalization(t *testing.T) {
	root := repoRoot(t)

	forbidden := []string{
		"normalizeEffort",
		"NormalizeEffort",
		"effortMap",
		"EffortMap",
		"effortAliases",
		"EffortAliases",
		"translateEffort",
		"TranslateEffort",
		"mapEffort",
		"MapEffort",
		"canonicalEffort",
		"CanonicalEffort",
		"universalEffort",
		"UniversalEffort",
	}

	scanRoots := []string{"cmd", "internal"}
	for _, sr := range scanRoots {
		base := filepath.Join(root, sr)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			rel, _ := filepath.Rel(root, path)
			for _, sym := range forbidden {
				if strings.Contains(text, sym) {
					t.Errorf("%s: contains forbidden symbol %q (R-XXWU-XUUJ: no cross-provider effort normalization layer)", rel, sym)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", sr, err)
		}
	}
}
