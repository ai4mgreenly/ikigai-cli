package scope_test

// R-UXDS-W9UQ: every line ikigai-cli writes to stdout must be a valid
// Claude Code stream-json event (assistant, user, result, system,
// rate_limit_event, or a forward-compatible variant). The wire layer
// is the only source of those lines: wire.Session / wire.Encode build
// the JSON object and emit it as a single newline-terminated line.
//
// To keep that invariant true by construction, cmd/ — the only
// package permitted to touch os.Stdout (R-BUFE-M5E0) — must not write
// to stdout directly. It hands os.Stdout to wire.* and lets that
// package own every byte. This static test forbids the obvious direct
// write tokens in non-test cmd/ source, so a future regression that
// prints a bare "hello\n" to stdout fails the suite instead of
// shipping a non-stream-json line.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_UXDS_W9UQ_StdoutOnlyViaWirePackage(t *testing.T) {
	root := repoRoot(t)

	// Tokens that write directly to stdout, bypassing wire.*.
	// fmt.Print/Println/Printf default to os.Stdout; the builtin
	// print/println go to stderr in the Go runtime but are still
	// banned to keep cmd/ output strictly intentional.
	forbidden := []string{
		"os.Stdout.Write",
		"os.Stdout.WriteString",
		"fmt.Fprint(os.Stdout",
		"fmt.Fprintln(os.Stdout",
		"fmt.Fprintf(os.Stdout",
		"fmt.Print(",
		"fmt.Println(",
		"fmt.Printf(",
		"bufio.NewWriter(os.Stdout)",
		"io.WriteString(os.Stdout",
	}

	cmdRoot := filepath.Join(root, "cmd")
	err := filepath.Walk(cmdRoot, func(path string, info os.FileInfo, err error) error {
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
		for _, tok := range forbidden {
			if strings.Contains(text, tok) {
				t.Errorf("%s: forbidden direct-stdout token %q (R-UXDS-W9UQ: stdout writes must go through internal/wire)", rel, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk cmd: %v", err)
	}
}
