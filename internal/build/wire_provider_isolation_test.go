package build_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestR_G0EH_D2SW_WireDoesNotImportProvider asserts that the
// wire-format codec stays provider-agnostic: no file under
// internal/wire/ may import any internal/provider/... package.
//
// R-G0EH-D2SW: "the agent loop and the wire-format codec do not
// import provider-specific packages." The interface lives in
// internal/provider/; concrete backends live under
// internal/provider/<name>/. Either kind of import from wire would
// couple the operator-facing wire format to a model contract.
func TestR_G0EH_D2SW_WireDoesNotImportProvider(t *testing.T) {
	root := repoRoot(t)
	wireDir := filepath.Join(root, "internal", "wire")

	const forbidden = "github.com/ai4mgreenly/ikigai-cli/internal/provider"

	fset := token.NewFileSet()
	err := filepath.Walk(wireDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == forbidden || strings.HasPrefix(p, forbidden+"/") {
				t.Errorf("%s imports %q (R-G0EH-D2SW: wire codec must stay provider-agnostic)", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", wireDir, err)
	}
}
