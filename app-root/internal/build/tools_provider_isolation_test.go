package build_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestR_WQ0H_645J_ToolsDoNotImportProvider asserts that tool
// implementations stay provider-agnostic: no file under
// internal/tools/ may import any internal/provider/... package.
//
// R-WQ0H-645J: "v1 supports three providers: Anthropic, OpenAI,
// and Google Gemini. Adding a fourth provider must not require
// changes to the wire-format translation layer or to existing
// tool implementations." R-G0EH-D2SW already covers the wire
// codec; this test extends the same isolation rule to tools so
// a future fourth provider is a pure additive change under
// internal/provider/<name>/.
func TestR_WQ0H_645J_ToolsDoNotImportProvider(t *testing.T) {
	root := repoRoot(t)
	toolsDir := filepath.Join(root, "internal", "tools")

	const forbidden = "github.com/ai4mgreenly/ikigai-cli/internal/provider"

	fset := token.NewFileSet()
	err := filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
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
				t.Errorf("%s imports %q (R-WQ0H-645J: tool implementations must stay provider-agnostic so adding a fourth provider does not require editing them)", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", toolsDir, err)
	}
}
