package scope_test

// R-S04B-QD3D: v1 implements Anthropic only, with the Read and Bash
// tools only. OpenAI and Google providers, and any non-MVP tool
// (Edit, Write, Grep, Glob, WebFetch, WebSearch, ...), are
// deliberately out of scope. The provider abstraction must be
// shaped so adding the others later does not require re-architecting
// the agent loop, the wire-format codec, the tool runtime, or the
// provider interface — but no implementation packages for the
// deferred providers or tools may exist in v1.
//
// This test enforces the rule statically by checking that no package
// directory exists for a non-MVP provider or tool.

import (
	"os"
	"path/filepath"
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

func TestR_S04B_QD3D_MVPScope(t *testing.T) {
	root := repoRoot(t)

	// Package directories that, if present, indicate a non-MVP
	// provider or tool has been built in v1.
	forbidden := []string{
		// Deferred providers.
		"internal/openai",
		"internal/google",
		"internal/gemini",
		// Non-MVP tools.
		"internal/edit",
		"internal/write",
		"internal/grep",
		"internal/glob",
		"internal/webfetch",
		"internal/websearch",
		"internal/task",
	}

	for _, rel := range forbidden {
		path := filepath.Join(root, rel)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			t.Errorf("%s: forbidden package directory exists (R-S04B-QD3D: v1 is Anthropic + Read + Bash only)", rel)
		}
	}
}
