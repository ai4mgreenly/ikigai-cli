package scope_test

// R-321O-P7TE: subagent / Task tool, NotebookEdit, background-bash
// lifecycle (BashOutput, KillBash), SlashCommand, Skill, WebFetch,
// WebSearch, and TodoWrite are not part of v1.
//
// Enforced statically by:
//   1. forbidding package directories under internal/ or cmd/ that
//      would house an implementation of any of the deferred tools;
//   2. forbidding the canonical tool-name identifiers from appearing
//      in any non-test source file under cmd/ or internal/.
//
// scope_test.go (R-S04B-QD3D) already forbids
// internal/{webfetch,websearch,task}; this test extends coverage to
// the rest of the deferred set and adds the identifier-token check.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_321O_P7TE_NoDeferredTools(t *testing.T) {
	root := repoRoot(t)

	forbiddenDirs := []string{
		"internal/tools/task",
		"internal/tools/subagent",
		"internal/tools/notebookedit",
		"internal/tools/bashoutput",
		"internal/tools/killbash",
		"internal/tools/slashcommand",
		"internal/tools/skill",
		"internal/tools/webfetch",
		"internal/tools/websearch",
		"internal/tools/todowrite",
		"internal/notebookedit",
		"internal/slashcommand",
		"internal/skill",
		"internal/todowrite",
		"cmd/task",
		"cmd/notebookedit",
	}
	for _, rel := range forbiddenDirs {
		path := filepath.Join(root, rel)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			t.Errorf("%s: forbidden package directory exists (R-321O-P7TE: deferred tool not in v1)", rel)
		}
	}

	forbiddenTokens := []string{
		"NotebookEdit",
		"BashOutput",
		"KillBash",
		"SlashCommand",
		"WebFetch",
		"WebSearch",
		"TodoWrite",
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
			for _, tok := range forbiddenTokens {
				if strings.Contains(text, tok) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s: references %q (R-321O-P7TE: deferred tool not in v1)", rel, tok)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", sr, err)
		}
	}
}
