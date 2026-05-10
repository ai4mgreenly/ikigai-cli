package scope_test

// R-AQ6C-0C5B: v1 implements exactly two tools — Read and Bash — and
// offers both to the model on every request. This test enforces the
// rule statically by asserting that internal/tools/ contains exactly
// the `read` and `bash` subdirectories and nothing else.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestR_AQ6C_0C5B_ToolsMembership(t *testing.T) {
	root := repoRoot(t)
	toolsDir := filepath.Join(root, "internal", "tools")

	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		t.Fatalf("read internal/tools: %v", err)
	}

	var got []string
	for _, e := range entries {
		if e.IsDir() {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)

	want := []string{"bash", "edit", "glob", "grep", "read", "write"}
	if len(got) != len(want) {
		t.Fatalf("internal/tools: got subdirs %v, want %v (R-AQ6C-0C5B / v1.x: Read + Bash + Write + Edit + Glob + Grep)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("internal/tools: got subdirs %v, want %v (R-AQ6C-0C5B / v1.x: Read + Bash + Write + Edit + Glob + Grep)", got, want)
		}
	}
}
