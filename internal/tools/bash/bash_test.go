package bash_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai4mgreenly/ikigai-cli/internal/tools/bash"
)

// R-JBSB-OY94: Bash runs in the foreground only. Run must block until
// the subprocess exits before returning the tool_result, so the model
// only sees the result after completion. Verified by running a command
// that sleeps measurably and asserting (a) the post-sleep output is
// present in the returned content, and (b) elapsed wall time is at
// least the sleep duration.
func TestR_JBSB_OY94_BashForegroundOnly(t *testing.T) {
	const useID = "toolu_bash_fg"
	const sleepFor = 200 * time.Millisecond
	start := time.Now()
	res, err := bash.Run(useID, "sleep 0.2; echo done")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true; want false")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, "done") {
		t.Fatalf("content missing post-sleep output; got %q", content)
	}
	if elapsed < sleepFor {
		t.Fatalf("Run returned in %v, less than sleep %v — did not block", elapsed, sleepFor)
	}
}

// R-JWIM-71UX: Bash enforces a per-invocation timeout. On timeout
// the process group is killed (so children spawned by the bash -c
// command are also torn down) and the returned tool_result is an
// error indicating the timeout. We shrink the timeout via a test
// hook so the suite doesn't wait 120s.
func TestR_JWIM_71UX_BashTimeout(t *testing.T) {
	restore := bash.SetTimeoutForTest(150 * time.Millisecond)
	defer restore()
	const useID = "toolu_bash_timeout"
	start := time.Now()
	res, err := bash.Run(useID, "sleep 5; echo should-not-appear")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.IsError {
		t.Fatalf("IsError = false; want true on timeout")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(strings.ToLower(content), "timed out") {
		t.Fatalf("content missing timeout marker; got %q", content)
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("Run waited %v; expected fast timeout", elapsed)
	}
}

// R-IR21-6UNB: Bash runs `bash -c <cmd>` and returns combined
// stdout+stderr as the tool_result content.
func TestR_IR21_6UNB_BashCombinedOutput(t *testing.T) {
	const useID = "toolu_bash_1"
	res, err := bash.Run(useID, "echo hello-out; echo hello-err 1>&2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Type != "tool_result" {
		t.Fatalf("Type = %q, want tool_result", res.Type)
	}
	if res.ToolUseID != useID {
		t.Fatalf("ToolUseID = %q, want %q", res.ToolUseID, useID)
	}
	if res.IsError {
		t.Fatalf("IsError = true; want false for successful command")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, "hello-out") || !strings.Contains(content, "hello-err") {
		t.Fatalf("content missing combined stdout+stderr; got %q", content)
	}
}

// R-LBQE-9F03: Bash surfaces the subprocess exit code in the
// tool_result body and treats non-zero exit as data, not is_error.
func TestR_LBQE_9F03_BashExitCodeInBody(t *testing.T) {
	const useID = "toolu_bash_exit"
	res, err := bash.Run(useID, "echo before-exit; exit 3")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true; non-zero exit must not set is_error")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, "before-exit") {
		t.Fatalf("content missing stdout; got %q", content)
	}
	if !strings.Contains(content, "[exit: 3]") {
		t.Fatalf("content missing exit code marker; got %q", content)
	}

	res2, err := bash.Run(useID, "true")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var content2 string
	if err := json.Unmarshal(res2.Content, &content2); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content2, "[exit: 0]") {
		t.Fatalf("content missing exit 0 marker; got %q", content2)
	}
}

// R-KM4I-88FI: Bash runs in the session cwd — the working directory
// ikigai-cli was launched in. The subprocess must inherit the parent
// process cwd; we verify by chdir-ing the test process to a tempdir
// and asserting `pwd` reports that same path.
func TestR_KM4I_88FI_BashRunsInSessionCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	const useID = "toolu_bash_cwd"
	res, err := bash.Run(useID, "pwd")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true; want false")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// macOS tempdirs resolve through /private; compare via realpath.
	real, err := filepath.EvalSymlinks(wd)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if !strings.Contains(content, wd) && !strings.Contains(content, real) {
		t.Fatalf("pwd output %q does not contain session cwd %q (real %q)", content, wd, real)
	}
}

// R-LXOL-5ACL: Bash truncates combined output exceeding 30000 bytes
// and appends a visible truncation notice (not silent).
func TestR_LXOL_5ACL_BashOutputTruncated(t *testing.T) {
	const useID = "toolu_bash_trunc"
	// Emit ~40000 bytes via head from /dev/zero piped through tr.
	res, err := bash.Run(useID, "head -c 40000 /dev/zero | tr '\\0' x")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true; want false")
	}
	var content string
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if !strings.Contains(content, "[truncated:") {
		t.Fatalf("content missing truncation notice; got len=%d", len(content))
	}
	if !strings.Contains(content, "[exit: 0]") {
		t.Fatalf("content missing exit marker after truncation; got %q", content[len(content)-200:])
	}
	// Body shape: 30000 bytes of output + "\n[truncated: output exceeded 30000 bytes]\n[exit: 0]".
	// The output portion must be capped; total should be modest beyond 30000.
	if len(content) > 30000+200 {
		t.Fatalf("content not bounded; len=%d", len(content))
	}

	// Sanity: short output gets no truncation notice.
	res2, err := bash.Run(useID, "echo short")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var content2 string
	if err := json.Unmarshal(res2.Content, &content2); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if strings.Contains(content2, "[truncated:") {
		t.Fatalf("short output should not carry truncation notice; got %q", content2)
	}
}
