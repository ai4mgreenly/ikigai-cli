package scope_test

// R-XXXX-XXXX is the literal placeholder string used in
// reqs/INTERACTIVE.md to document the requirement-ID FORMAT
// ("IDs of the form R-XXXX-XXXX"). It is not a real requirement.
// This test guards that fact: the literal must appear ONLY in
// format-explanation prose, never as a tagged requirement (i.e.
// never on a line that starts with the conventional
// "- R-XXXX-XXXX:" or "R-XXXX-XXXX:" tagging pattern), and must
// not be referenced by any non-test source file in the workdir.
//
// Indented lines (>=4 leading spaces or a leading tab) are treated
// as Markdown code/example blocks and skipped — those are illustrative
// renderings of the tag format, not live requirement tags.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_XXXX_XXXX_PlaceholderNotRequirement(t *testing.T) {
	root := repoRoot(t)
	const literal = "R-XXXX-XXXX"

	reqsDir := filepath.Join(root, "reqs")
	err := filepath.Walk(reqsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if !strings.Contains(line, literal) {
				continue
			}
			if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
				continue
			}
			trimmed := strings.TrimLeft(line, " \t-*")
			if strings.HasPrefix(trimmed, literal+":") {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d: %q used as a real requirement tag; it is reserved as the format placeholder", rel, lineNo, literal)
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatalf("walk reqs: %v", err)
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
			if strings.Contains(string(data), literal) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s: references %q; that string is the format placeholder, not a real requirement", rel, literal)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", sr, err)
		}
	}
}
