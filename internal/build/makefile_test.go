package build_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot returns the repository root by walking upward from the
// current working directory until a Makefile is found.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root with Makefile + go.mod")
		}
		dir = parent
	}
}

// targetDeps parses a Makefile and returns the prerequisite list for
// the named target. Returns (nil, false) if the target is missing.
func targetDeps(makefile, target string) ([]string, bool) {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `[ \t]*:([^=\n][^\n]*)?$`)
	m := re.FindStringSubmatch(makefile)
	if m == nil {
		return nil, false
	}
	rhs := strings.TrimSpace(m[1])
	if rhs == "" {
		return []string{}, true
	}
	return strings.Fields(rhs), true
}

// TestR_J5PD_8EBD_MakefileTargets verifies the repo root Makefile has
// the four required targets and that `clean` does not depend on a
// build step.
func TestR_J5PD_8EBD_MakefileTargets(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	mk := string(data)

	for _, target := range []string{"build", "test", "install", "clean"} {
		if _, ok := targetDeps(mk, target); !ok {
			t.Errorf("Makefile is missing required target %q", target)
		}
	}

	// clean must not transitively depend on build. With the Makefile
	// kept simple (no aliasing), a direct check on its prerequisite
	// list is sufficient.
	deps, ok := targetDeps(mk, "clean")
	if !ok {
		return // already reported above
	}
	for _, d := range deps {
		if d == "build" || strings.HasPrefix(d, "$(BIN") {
			t.Errorf("clean target depends on %q; R-J5PD-8EBD requires clean to be independent of build", d)
		}
	}

	// build target must exist as default (first .PHONY/real target).
	// We just assert build appears before clean in the file as a
	// proxy for "default target is build".
	if strings.Index(mk, "\nbuild") > strings.Index(mk, "\nclean") {
		t.Errorf("build target should appear before clean so `make` with no args runs build")
	}
}
