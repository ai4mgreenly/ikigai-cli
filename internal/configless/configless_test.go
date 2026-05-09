package configless_test

// R-7IWS-GMJF: ikigai-cli is configless. No config file, no
// ~/.ikigai/ directory, no IKIGAI_* environment variables, no
// --config flag, no XDG_* lookups, no autoload of any settings
// file. The only environment variables the binary may read are the
// provider API keys defined in R-YL2Y-7HXQ
// (ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY).
//
// This test enforces the rule statically by scanning every
// non-test .go file in the tree for forbidden tokens.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (no go.mod found)")
		}
		dir = parent
	}
}

func TestR_7IWS_GMJF_Configless(t *testing.T) {
	root := repoRoot(t)

	// Substrings that, if present in a non-test source file,
	// indicate a configless-rule violation.
	forbidden := []string{
		"IKIGAI_",   // no IKIGAI_* env vars
		"--config",  // no --config flag
		"\"config\"", // no flag named "config"
		"XDG_",      // no XDG_* lookups
		".ikigai",   // no ~/.ikigai paths
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "bin" || name == ".ralph" || name == "reqs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// This very test file mentions the forbidden tokens by
		// design; skip all _test.go files. Runtime behavior is
		// covered by other tests.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		rel, _ := filepath.Rel(root, path)
		for _, tok := range forbidden {
			if strings.Contains(src, tok) {
				t.Errorf("%s: forbidden token %q (R-7IWS-GMJF: ikigai-cli is configless)", rel, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
