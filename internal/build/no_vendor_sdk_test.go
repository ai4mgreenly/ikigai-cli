package build_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestR_W5A6_O0JQ_NoVendorSDK asserts that no vendor LLM-provider SDK
// module is declared in go.mod. ikigai-cli must speak to provider
// HTTP APIs directly; general-purpose libraries (HTTP, SSE, JSON,
// CLI, fs) are fine.
//
// R-W5A6-O0JQ: ikigai-cli speaks to provider HTTP APIs directly. No
// vendor SDK (Anthropic, OpenAI, Google) may be linked.
func TestR_W5A6_O0JQ_NoVendorSDK(t *testing.T) {
	root := repoRoot(t)

	// Module-path prefixes for vendor LLM-provider SDKs that are
	// forbidden as v1 dependencies. Match by prefix so any
	// sub-package counts.
	forbidden := []string{
		"github.com/anthropics/anthropic-sdk-go",
		"github.com/sashabaranov/go-openai",
		"github.com/openai/openai-go",
		"google.golang.org/genai",
		"cloud.google.com/go/vertexai",
		"cloud.google.com/go/aiplatform",
		"github.com/google/generative-ai-go",
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && name == "go.sum" {
				continue
			}
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(data)
		for _, mod := range forbidden {
			if strings.Contains(text, mod) {
				t.Errorf("%s references forbidden vendor SDK %q (R-W5A6-O0JQ: no vendor LLM SDK)", name, mod)
			}
		}
	}
}
