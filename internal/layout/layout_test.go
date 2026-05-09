package layout_test

// R-BUFE-M5E0: no package outside cmd/ may read os.Args, touch
// os.Stdin/os.Stdout directly, or install signal handlers. All such
// wiring must live in cmd/ikigai-cli; library packages take callers'
// io.Reader/io.Writer, context.Context, and config values.

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

func TestR_BUFE_M5E0_NoDirectIOOutsideCmd(t *testing.T) {
	root := repoRoot(t)

	// Tokens that indicate forbidden direct-IO or process wiring.
	// Library code must accept these via parameters instead.
	forbidden := []string{
		"os.Args",
		"os.Stdin",
		"os.Stdout",
		"os.Stderr",
		"signal.Notify",
		"signal.NotifyContext",
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			// Skip cmd/ — that is the only place wiring is allowed.
			if path == filepath.Join(root, "cmd") {
				return filepath.SkipDir
			}
			// Skip non-source dirs.
			if name == ".git" || name == "bin" || name == ".ralph" || name == "reqs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Tests in library packages may legitimately use these for
		// scaffolding (this very file does), so skip _test.go.
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
				t.Errorf("%s: forbidden reference %q in non-cmd package (R-BUFE-M5E0)", rel, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
