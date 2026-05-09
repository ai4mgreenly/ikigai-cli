// Package bash implements the Bash tool exposed to the model.
package bash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/ai4mgreenly/ikigai-cli/internal/wire"
)

// R-YNXM-CVXI: the tool's exposed name and input JSON schema match
// Claude Code's built-in Bash tool for the supported MVP subset
// (just `command`). Background execution, custom timeouts, sandbox
// flags, and descriptions are intentionally absent — ikigai-cli's
// Bash runs a fixed-policy foreground subprocess only.
const Name = "Bash"

// InputSchema is the JSON Schema advertised to the model for the
// Bash tool. Shape matches Claude Code's Bash schema for the
// supported subset.
var InputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The command to execute"
    }
  },
  "required": ["command"]
}`)

// Run executes cmd via `bash -c` and returns a tool_result block
// correlated to toolUseID containing the combined stdout+stderr
// followed by the subprocess exit code.
//
// R-IR21-6UNB: the Bash tool runs the command in a `bash -c`
// subprocess and returns combined stdout+stderr. The command is not
// parsed or sanitized; whatever the model supplies is what runs.
// R-LBQE-9F03: the exit code is appended to the content as a final
// `[exit: N]` line; non-zero exit is not an is_error — it is data.
// R-LXOL-5ACL: combined output is truncated to maxOutputBytes; when
// truncation occurs a visible `[truncated: ...]` notice is appended
// between the (truncated) output and the `[exit: N]` line.
// R-KM4I-88FI: the subprocess inherits the session cwd (the working
// directory ikigai-cli was launched in). exec.Command leaves Cmd.Dir
// empty by default, which causes the child to inherit the parent's
// cwd — that is precisely what this requirement specifies.
// R-JBSB-OY94: Bash runs in the foreground only — Run blocks on
// CombinedOutput until the subprocess exits, so the model only sees
// the result after completion. There is no background/async path.
// R-JWIM-71UX: each invocation is bounded by bashTimeout (120s by
// default). Setpgid puts the child in its own process group so a
// timeout can SIGKILL the whole group (-pgid), tearing down any
// grandchildren bash forked. On timeout Run returns an is_error
// tool_result indicating the timeout.
func Run(toolUseID, cmd string) (wire.ToolResultBlock, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	out, err := c.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return wire.NewToolResultBlock(toolUseID, true,
			fmt.Sprintf("Bash: command timed out after %s", bashTimeout))
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return wire.NewToolResultBlock(toolUseID, true, fmt.Sprintf("Bash: %v", err))
		}
	}
	truncated := false
	if len(out) > maxOutputBytes {
		out = out[:maxOutputBytes]
		truncated = true
	}
	var body string
	if truncated {
		body = fmt.Sprintf("%s\n[truncated: output exceeded %d bytes]\n[exit: %d]",
			string(out), maxOutputBytes, c.ProcessState.ExitCode())
	} else {
		body = fmt.Sprintf("%s\n[exit: %d]", string(out), c.ProcessState.ExitCode())
	}
	return wire.NewToolResultBlock(toolUseID, false, body)
}

const maxOutputBytes = 30000

var bashTimeout = 120 * time.Second
