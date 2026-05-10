package scope_test

// R-Z5T8-PLJJ: ikigai-cli implements Claude Code's built-in tools
// itself and exposes them to the underlying model via that
// provider's native tool-use mechanism. The Anthropic backend is
// NOT a passthrough to the real `claude` binary — it uses the
// Messages API and runs tools locally, same as the other backends.
//
// This test enforces the no-passthrough rule statically by
// scanning non-test source for tokens that would indicate shelling
// out to a `claude` executable.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_Z5T8_PLJJ_NoClaudeBinaryPassthrough(t *testing.T) {
	root := repoRoot(t)

	forbidden := []string{
		`exec.Command("claude"`,
		`exec.CommandContext("claude"`, // covers ctx variant
		`LookPath("claude")`,
		`exec.LookPath("claude")`,
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
			for _, tok := range forbidden {
				if strings.Contains(text, tok) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: contains forbidden token %q (R-Z5T8-PLJJ: no passthrough to real claude binary)", rel, tok)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", sr, err)
		}
	}
}
