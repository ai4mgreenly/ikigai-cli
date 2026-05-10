package scope_test

// R-2DNP-1SZI: MCP servers are not exposed as tools.
// `ENABLE_CLAUDEAI_MCP_SERVERS` and `CLAUDE_CONFIG_DIR` are accepted
// and ignored — consistent with the configless rule.
//
// This test enforces the rule statically by:
//   1. forbidding any MCP-related package directory under internal/
//      or cmd/ (an MCP integration would have to live in a package);
//   2. forbidding MCP-related identifiers and the two env-var names
//      from appearing in any non-test source file. Since the binary
//      never reads those env vars (the configless rule already
//      forbids env-var reads beyond provider API keys), "accepted
//      and ignored" reduces to "the names are not referenced in
//      code at all."

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR_2DNP_1SZI_NoMCP(t *testing.T) {
	root := repoRoot(t)

	forbiddenDirs := []string{
		"internal/mcp",
		"internal/tools/mcp",
		"cmd/mcp",
	}
	for _, rel := range forbiddenDirs {
		path := filepath.Join(root, rel)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			t.Errorf("%s: forbidden package directory exists (R-2DNP-1SZI: MCP is not part of v1)", rel)
		}
	}

	forbiddenTokens := []string{
		"ENABLE_CLAUDEAI_MCP_SERVERS",
		"CLAUDE_CONFIG_DIR",
		"ModelContextProtocol",
		"mcp_server",
		"MCPServer",
		"mcpServer",
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
					t.Errorf("%s: references %q (R-2DNP-1SZI: MCP is not part of v1; the named env vars must be accepted and ignored, i.e. not read)", rel, tok)
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", sr, err)
		}
	}
}
